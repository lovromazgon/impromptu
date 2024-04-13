package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	logger := log.NewLogfmtLogger(os.Stderr)

	cfg := config.DefaultConfig
	cfg.GlobalConfig.ScrapeInterval = model.Duration(1 * time.Second)

	scrapeCfg := config.DefaultScrapeConfig
	scrapeCfg.JobName = "cli"
	cfg.ScrapeConfigs = []*config.ScrapeConfig{&scrapeCfg}

	db, err := tsdb.Open(
		"./.impromptu_data",
		log.With(logger, "component", "tsdb"),
		prometheus.DefaultRegisterer,
		tsdb.DefaultOptions(),
		nil,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening storage: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	mgr, err := scrape.NewManager(
		&scrape.Options{},
		log.With(logger, "component", "scrape manager"),
		db,
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating scrape manager: %v\n", err)
		os.Exit(1)
	}
	err = mgr.ApplyConfig(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error applying config: %v\n", err)
		os.Exit(1)
	}

	ch := make(chan map[string][]*targetgroup.Group)
	go func() {
		err := mgr.Run(ch)
		if err != nil {
			_ = logger.Log("msg", "error running scrape manager", "err", err)
			cancel()
		}
	}()
	defer mgr.Stop()

	l := labels.FromMap(map[string]string{
		model.AddressLabel:     "localhost:8080",
		model.SchemeLabel:      "http",
		model.MetricsPathLabel: "/metrics",
	})
	target := scrape.NewTarget(l, l, nil)
	fmt.Println(target.URL().String())

	targetGroups := map[string][]*targetgroup.Group{
		"cli": {
			&targetgroup.Group{
				Targets: []model.LabelSet{{
					model.AddressLabel:     model.LabelValue("localhost:8080"),
					model.SchemeLabel:      model.LabelValue("http"),
					model.MetricsPathLabel: model.LabelValue("/metrics"),
				}},
				Labels: model.LabelSet{},
				Source: "cli",
			},
		},
	}
	ch <- targetGroups

	opts := promql.EngineOpts{
		Logger:             log.With(logger, "component", "query engine"),
		Reg:                prometheus.DefaultRegisterer,
		MaxSamples:         50000000,
		Timeout:            time.Minute,
		ActiveQueryTracker: promql.NewActiveQueryTracker("./.impromptu_data", 20, log.With(logger, "component", "activeQueryTracker")),
		LookbackDelta:      time.Minute * 5,
		NoStepSubqueryIntervalFn: func(_ int64) int64 {
			return int64(time.Duration(cfg.GlobalConfig.EvaluationInterval) / time.Millisecond)
		},
		// EnableAtModifier and EnableNegativeOffset have to be
		// always on for regular PromQL as of Prometheus v2.33.
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   false,
	}
	engine := promql.NewEngine(opts)

	for {
		select {
		case <-time.After(time.Second):
			fmt.Println("---------------------")
			now := time.Now()
			q, err := engine.NewRangeQuery(ctx, db, promql.NewPrometheusQueryOpts(false, time.Minute*5), `{__name__!=""}`, now.Add(-time.Second*3), now, time.Second)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating query: %v\n", err)
				os.Exit(1)
			}
			defer q.Close()
			r := q.Exec(ctx)
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "error executing query: %v\n", r.Err)
				os.Exit(1)
			}
			fmt.Println(r.String())
		case <-ctx.Done():
			fmt.Println("goodbye")
			return
		}
	}
}
