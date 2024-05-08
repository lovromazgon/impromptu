package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/lovromazgon/impromptu"
	"github.com/lovromazgon/impromptu/opt"
)

const usage = `Usage:
    impromptu -t URL -q PROMQL_QUERY [-i DURATION] [-r DURATION]

Options:
    -t, --target-url URL               Fetch metrics from the specified URL
    -q, --query-string PROMQL_QUERY    Query to execute against the metrics
    -i, --query-interval DURATION      Interval to fetch metrics at [default: 1s]
    -r, --query-range DURATION         Range of the query [default: 5m]

URL represents an endpoint that serves Prometheus metrics in text format.

PROMQL_QUERY is a Prometheus query language expression that returns a single
time series. The query is executed every second and the result is displayed
in a terminal chart. The query should return a single time series, e.g. a
rate or a sum of a counter. The query interval and range can be adjusted with
the -i and -r flags.

DURATION is a time duration string that can be parsed by Go's time.ParseDuration
function. It represents a time interval, e.g. "5m" for 5 minutes, "1h" for 1
hour, "30s" for 30 seconds, "1h 2m 3s" for 1 hour, 2 minutes and 3 seconds etc.

Example:
    $ impromptu -t http://demo.do.prometheus.io:9100/metrics -q "rate(node_cpu_seconds_total{mode=\"idle\"}[5s])"`

func main() {
	opts := parseOptions()

	cleanup := initStderr()
	var err error
	defer func() {
		cleanup()
		if err != nil {
			log.Fatal(err)
		}
	}()

	i, err := impromptu.New(opts)
	if err != nil {
		return
	}

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	err = i.Run(ctx)
}

func parseOptions() opt.Options {
	var (
		targetURL     string
		queryString   string
		queryInterval time.Duration
		queryRange    time.Duration
	)

	flag.StringVar(&targetURL, "target-url", "", "Fetch metrics from the specified URL")
	flag.StringVar(&targetURL, "t", "", "Fetch metrics from the specified URL")
	flag.StringVar(&queryString, "query-string", "", "Query to execute against the metrics")
	flag.StringVar(&queryString, "q", "", "Query to execute against the metrics")
	flag.DurationVar(&queryInterval, "query-interval", opt.Defaults.QueryInterval, "Interval to fetch metrics at")
	flag.DurationVar(&queryInterval, "i", opt.Defaults.QueryInterval, "Interval to fetch metrics at")
	flag.DurationVar(&queryRange, "query-range", opt.Defaults.QueryRange, "Range of the query")
	flag.DurationVar(&queryRange, "r", opt.Defaults.QueryRange, "Range of the query")
	flag.Usage = func() { _, _ = fmt.Fprintf(os.Stderr, "%s\n", usage) }
	flag.Parse()

	if targetURL == "" || queryString == "" {
		flag.Usage()
		os.Exit(1)
	}

	return opt.Options{
		TargetURL:      targetURL,
		ScrapeInterval: queryInterval, // same as query interval
		QueryString:    queryString,
		QueryRange:     queryRange,
		QueryInterval:  queryInterval,
	}
}

func initStderr() func() {
	cleanup := func() {}

	oldStderr := os.Stderr
	err := os.MkdirAll(opt.DataPath, 0o664)
	if err != nil {
		log.Printf("error creating data directory, logs will be written to stderr (can result in glitches): %v", err)
		return cleanup
	}
	f, err := os.OpenFile(opt.DataPath+"/impromptu.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o664)
	if err != nil {
		log.Printf("error opening log file, logs will be written to stderr (can result in glitches): %v", err)
		return cleanup
	}
	os.Stderr = f
	cleanup = func() {
		os.Stderr = oldStderr
		err := f.Close()
		if err != nil {
			log.Printf("error closing log file: %v", err)
		}
	}
	return cleanup
}
