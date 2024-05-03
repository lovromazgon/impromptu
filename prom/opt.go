package prom

import (
	"io"
	"os"
	"time"
)

type options struct {
	loggerWriter       io.Writer
	scrapeInterval     time.Duration
	evaluationInterval time.Duration
	targetURL          string

	queryString   string
	queryRange    time.Duration
	queryInterval time.Duration
}

var defaultOptions = options{
	loggerWriter:       os.Stderr,
	scrapeInterval:     1 * time.Second,
	evaluationInterval: 1 * time.Minute,
	// targetURL:          "http://localhost:8080/metrics",

	queryRange:    time.Minute * 5,
	queryInterval: time.Second,
}

type Opt interface {
	opt(*options)
}

type optFunc func(*options)

func (f optFunc) opt(o *options) {
	f(o)
}

func WithScrapeInterval(interval time.Duration) Opt {
	return optFunc(func(o *options) {
		o.scrapeInterval = interval
	})
}

func WithLoggerWriter(w io.Writer) Opt {
	return optFunc(func(o *options) {
		o.loggerWriter = w
	})
}

func WithEvaluationInterval(interval time.Duration) Opt {
	return optFunc(func(o *options) {
		o.evaluationInterval = interval
	})
}

func WithTargetURL(url string) Opt {
	return optFunc(func(o *options) {
		o.targetURL = url
	})
}

func WithQuery(qStr string, qRange time.Duration, qInterval time.Duration) Opt {
	return optFunc(func(o *options) {
		o.queryString = qStr
		if qRange != 0 {
			o.queryRange = qRange
		}
		if qInterval != 0 {
			o.queryInterval = qInterval
		}
	})
}
