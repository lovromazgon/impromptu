package opt

import (
	"time"
)

const DataPath = "/tmp/impromptu"

type Options struct {
	TargetURL   string
	QueryString string

	ScrapeInterval time.Duration
	QueryRange     time.Duration
	QueryInterval  time.Duration
}

var Defaults = Options{
	ScrapeInterval: time.Second,
	QueryRange:     time.Minute * 5,
	QueryInterval:  time.Second,
}
