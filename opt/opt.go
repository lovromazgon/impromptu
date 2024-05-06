package opt

import (
	"io"
	"os"
	"time"
)

type Options struct {
	LoggerWriter       io.Writer
	ScrapeInterval     time.Duration
	EvaluationInterval time.Duration
	TargetURL          string

	QueryString   string
	QueryRange    time.Duration
	QueryInterval time.Duration
}

var Defaults = Options{
	LoggerWriter:       os.Stderr,
	ScrapeInterval:     1 * time.Second,
	EvaluationInterval: 1 * time.Minute,
	// targetURL:          "http://localhost:8080/metrics",

	QueryRange:    time.Minute * 5,
	QueryInterval: time.Second,
}
