package prom

import (
	"context"
	"fmt"
	"github.com/lovromazgon/impromptu/opt"
	"net/url"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

type Prom struct {
	logger        log.Logger
	labels        model.LabelSet
	queryString   string
	queryRange    time.Duration
	queryInterval time.Duration

	tsdb          *tsdb.DB
	scrapeManager *scrape.Manager
	promqlEngine  *promql.Engine
	out           chan promql.Matrix
}

func New(options opt.Options) (*Prom, error) {
	logger := log.NewLogfmtLogger(options.LoggerWriter)

	cfg := config.DefaultConfig
	cfg.GlobalConfig.ScrapeInterval = model.Duration(options.ScrapeInterval)
	cfg.GlobalConfig.EvaluationInterval = model.Duration(options.EvaluationInterval)

	scrapeCfg := config.DefaultScrapeConfig
	scrapeCfg.JobName = "impromptu"
	cfg.ScrapeConfigs = []*config.ScrapeConfig{&scrapeCfg}

	promqlEngineOpts := promql.EngineOpts{
		Logger:             log.With(logger, "component", "query engine"),
		Reg:                prometheus.DefaultRegisterer,
		MaxSamples:         50000000,
		Timeout:            time.Minute,
		ActiveQueryTracker: NewSequentialQueryTracker(),
		LookbackDelta:      options.QueryRange,
		NoStepSubqueryIntervalFn: func(_ int64) int64 {
			return int64(time.Duration(cfg.GlobalConfig.EvaluationInterval) / time.Millisecond)
		},
		// EnableAtModifier and EnableNegativeOffset have to be
		// always on for regular PromQL as of Prometheus v2.33.
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   false,
	}

	targetURL, err := url.Parse(options.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing target URL: %w", err)
	}
	l := model.LabelSet{
		model.AddressLabel:     model.LabelValue(targetURL.Host),
		model.SchemeLabel:      model.LabelValue(targetURL.Scheme),
		model.MetricsPathLabel: model.LabelValue(targetURL.Path),
	}

	p := &Prom{
		logger: logger,
		labels: l,

		queryString:   options.QueryString,
		queryRange:    options.QueryRange,
		queryInterval: options.QueryInterval,
	}

	err = p.init(cfg, promqlEngineOpts)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Prom) init(cfg config.Config, promqlEngineOpts promql.EngineOpts) (err error) {
	db, err := tsdb.Open(
		"./.impromptu_data",
		log.With(p.logger, "component", "tsdb"),
		prometheus.DefaultRegisterer,
		tsdb.DefaultOptions(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("error opening storage: %w", err)
	}
	defer func() {
		if err != nil {
			if dbErr := db.Close(); dbErr != nil {
				_ = p.logger.Log("msg", "error closing storage", "err", dbErr)
			}
		}
	}()

	mgr, err := scrape.NewManager(
		&scrape.Options{},
		log.With(p.logger, "component", "scrape manager"),
		db,
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		return fmt.Errorf("error creating scrape manager: %w", err)
	}

	err = mgr.ApplyConfig(&cfg)
	if err != nil {
		return fmt.Errorf("error applying config: %w", err)
	}

	promqlEngine := promql.NewEngine(promqlEngineOpts)

	now := time.Now()
	q, err := promqlEngine.NewRangeQuery(
		context.Background(),
		db,
		promql.NewPrometheusQueryOpts(false, 0),
		p.queryString,
		now.Add(-p.queryRange),
		now,
		p.queryInterval,
	)
	if err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	q.Cancel()
	q.Close()

	p.tsdb = db
	p.scrapeManager = mgr
	p.promqlEngine = promqlEngine
	p.out = make(chan promql.Matrix)

	return nil
}

func (p *Prom) Run(ctx context.Context) error {
	ch := make(chan map[string][]*targetgroup.Group, 1)
	ch <- map[string][]*targetgroup.Group{
		"impromptu": {
			&targetgroup.Group{
				Targets: []model.LabelSet{p.labels},
				Labels:  model.LabelSet{},
				Source:  "impromptu",
			},
		},
	}

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return p.scrapeManager.Run(ch)
	})
	group.Go(func() error {
		<-ctx.Done()
		p.scrapeManager.Stop()
		if err := p.tsdb.Close(); err != nil {
			return fmt.Errorf("error closing storage: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		rateLimit := rate.NewLimiter(rate.Every(p.queryInterval), 1)
		var previousQuery promql.Query
		defer close(p.out)
		for {
			err := rateLimit.Wait(ctx)
			if err != nil {
				return err
			}
			q, err := p.execQuery(ctx)

			if previousQuery != nil {
				// assume that the previous query has been processed and can be closed
				previousQuery.Close()
			}
			previousQuery = q

			if err != nil {
				return err
			}
		}
	})

	return group.Wait()
}

// execQuery executes the query and sends the result to the output channel.
// It returns the query object so that it can be closed when the next query is
// executed.
func (p *Prom) execQuery(ctx context.Context) (promql.Query, error) {
	now := time.Now()
	q, err := p.promqlEngine.NewRangeQuery(
		ctx,
		p.tsdb,
		promql.NewPrometheusQueryOpts(false, 0),
		p.queryString,
		now.Add(-p.queryRange),
		now,
		p.queryInterval,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	r := q.Exec(ctx)
	if r.Err != nil {
		return nil, fmt.Errorf("error executing query: %w", r.Err)
	}
	m, err := r.Matrix()
	if err != nil {
		return nil, fmt.Errorf("error fetching result matrix: %w", r.Err)
	}
	if len(m) == 0 {
		q.Close()
		return nil, nil
	}

	select {
	case <-ctx.Done():
		q.Close()
		return nil, ctx.Err()
	case p.out <- m:
	}

	return q, nil
}

func (p *Prom) Out() <-chan promql.Matrix {
	return p.out
}
