package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/log"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
)

// playLineChart continuously adds values to the LineChart, once every delay.
// Exits when the context expires.
func drawLineChart(lc *linechart.LineChart, series promql.Series) {
	input := make([]float64, len(series.Floats))
	for i, f := range series.Floats {
		input[i] = f.F
	}
	if err := lc.Series("first", input,
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
		linechart.SeriesXLabels(map[int]string{
			0: "zero",
		}),
	); err != nil {
		panic(err)
	}
}

func dash(ctx context.Context, cancel context.CancelFunc, w widgetapi.Widget) {
	t, err := tcell.New()
	if err != nil {
		panic(err)
	}
	defer t.Close()

	const redrawInterval = 250 * time.Millisecond
	c, err := container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("PRESS Q TO QUIT"),
		container.PlaceWidget(w),
	)
	if err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
	}

	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(redrawInterval)); err != nil {
		panic(err)
	}
}

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

	lc, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorCyan)),
	)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			select {
			case <-time.After(time.Second):
				now := time.Now()
				q, err := engine.NewRangeQuery(ctx, db, promql.NewPrometheusQueryOpts(false, time.Minute*5), `rate(conduit_pipeline_execution_duration_seconds_count[1m])`, now.Add(-time.Minute*10), now, time.Second)
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
				m, err := r.Matrix()
				if err != nil {
					continue
				}
				if len(m) > 0 {
					drawLineChart(lc, m[0])
				}
			case <-ctx.Done():
				fmt.Println("goodbye")
				return
			}
		}
	}()

	dash(ctx, cancel, lc)
}
