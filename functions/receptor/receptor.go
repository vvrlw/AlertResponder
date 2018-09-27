package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/m-mizutani/AlertResponder/lib"
	"github.com/pkg/errors"
)

// Config is data structure for emitter main procedure
type Config struct {
	Region         string
	TaskStreamName string
	AlertMapName   string
	ReportTo       string
}

type ReceptorResponse struct {
	ReportIDs []string `json:"report_ids"`
}

func buildConfig(ctx context.Context) (*Config, error) {
	arn, err := lib.NewArnFromContext(ctx)
	if err != nil {
		return nil, err
	}

	cfg := Config{
		Region:         arn.Region(),
		AlertMapName:   os.Getenv("ALERT_MAP"),
		TaskStreamName: os.Getenv("STREAM_NAME"),
		ReportTo:       os.Getenv("REPORT_TO"),
	}

	return &cfg, nil
}

func ParseEvent(event events.KinesisEvent) ([]lib.Alert, error) {
	alerts := []lib.Alert{}

	for _, record := range event.Records {
		src := record.Kinesis.Data
		log.Println("data = ", string(src))

		alert := lib.Alert{}
		err := json.Unmarshal(src, &alert)
		if err != nil {
			log.Println("Invalid alert data: ", string(src))
			return alerts, errors.Wrap(err, "Invalid json format in KinesisRecord")
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

func alertToReport(cfg *Config, alert *lib.Alert) (*lib.Report, error) {
	lib.Dump("alert", alert)
	alertMap := NewAlertMap(cfg.AlertMapName, cfg.Region)

	reportID, err := alertMap.Lookup(alert.Key, alert.Rule)
	if err != nil {
		return nil, err
	}

	if reportID == nil {
		// Existing alert issue is not found
		reportID, err = alertMap.Create(alert.Key, alert.Rule)

		if err != nil {
			return nil, errors.Wrap(err, "Failt to create a new alert map")
		}
		log.Printf("Created a new reportDI: %s", *reportID)
	}

	report := lib.NewReport(*reportID, alert)

	return report, nil
}

// Handler is main logic of Emitter
func Handler(cfg Config, alerts []lib.Alert) ([]string, error) {
	log.Printf("Start handling %d alert(s)\n", len(alerts))
	resp := []string{}

	for _, alert := range alerts {
		report, err := alertToReport(&cfg, &alert)
		if err != nil {
			return resp, err
		}

		err = lib.ExecDelayMachine(os.Getenv("DISPATCH_MACHINE"), cfg.Region, report)
		if err != nil {
			return resp, errors.Wrap(err, "Fail to start DispatchMachine")
		}

		err = lib.ExecDelayMachine(os.Getenv("REVIEW_MACHINE"), cfg.Region, report)
		if err != nil {
			return resp, errors.Wrap(err, "Fail to start ReviewMachine")
		}

		err = lib.PublishSnsMessage(os.Getenv("REPORT_LINE"), cfg.Region, report)
		if err != nil {
			return resp, err
		}

		log.Println("put alert to task stream")
		resp = append(resp, string(report.ID))
	}

	return resp, nil
}

// HandleRequest is Lambda handler
func HandleRequest(ctx context.Context, event events.KinesisEvent) (ReceptorResponse, error) {
	lib.Dump("Event", event)

	var resp ReceptorResponse

	cfg, err := buildConfig(ctx)
	if err != nil {
		return resp, err
	}

	events, err := ParseEvent(event)
	if err != nil {
		return resp, err
	}

	ids, err := Handler(*cfg, events)
	if err != nil {
		return resp, err
	}

	resp.ReportIDs = ids
	return resp, nil
}

func main() {
	lambda.Start(HandleRequest)
}
