package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/m-mizutani/AlertResponder/lib"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type CompiledReport struct {
	Report *lib.Report `json:"report"`
}

type parameters struct {
	region    string
	tableName string
}

func buildParameters(ctx context.Context) (*parameters, error) {
	arn, err := lib.NewArnFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Fail to extract region from ARN")
	}

	params := parameters{
		region:    arn.Region(),
		tableName: os.Getenv("REPORT_DATA"),
	}

	return &params, nil
}

// HandleRequest is a main Lambda handler
func HandleRequest(ctx context.Context, report lib.Report) (*lib.Report, error) {
	log.WithField("report", report).Info("start")

	params, err := buildParameters(ctx)
	if err != nil {
		return nil, err
	}

	pages, err := lib.FetchReportPages(params.tableName, params.region, report.ID)
	if err != nil {
		return nil, err
	}

	log.WithField("pages", pages).Info("Fetched pages")

	c := &report.Content
	c.RemoteHosts = map[string]lib.ReportRemoteHost{}
	c.LocalHosts = map[string]lib.ReportLocalHost{}

	for _, page := range pages {
		for _, r := range page.RemoteHost {
			log.WithField("id", r.ID).Info("set section to remote")
			h, _ := c.RemoteHosts[r.ID]
			h.Merge(r)
			c.RemoteHosts[r.ID] = h
		}

		for _, r := range page.LocalHost {
			log.WithField("id", r.ID).Info("set section to local")
			h, _ := c.LocalHosts[r.ID]
			h.Merge(r)
			c.LocalHosts[r.ID] = h
		}
	}

	return &report, nil
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	switch os.Getenv("LOG_LEVEL") {
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	lambda.Start(HandleRequest)
}
