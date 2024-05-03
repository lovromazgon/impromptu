package impromptu

import (
	"context"
	"fmt"

	"github.com/lovromazgon/impromptu/dash"
	"github.com/lovromazgon/impromptu/prom"
	"golang.org/x/sync/errgroup"
)

// rate(conduit_pipeline_execution_duration_seconds_count[5s])

type Impromptu struct {
	prom *prom.Prom
	dash *dash.Dash
}

func New() (*Impromptu, error) {
	p, err := prom.New(
		prom.WithTargetURL("http://localhost:8080/metrics"),
		prom.WithQuery("rate(conduit_pipeline_execution_duration_seconds_count[5s])", 0, 0),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating prom: %w", err)
	}

	d, err := dash.New()
	if err != nil {
		return nil, fmt.Errorf("error creating dash: %w", err)
	}

	return &Impromptu{
		prom: p,
		dash: d,
	}, nil
}

func (i *Impromptu) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return i.prom.Run(ctx)
	})
	group.Go(func() error {
		return i.dash.Run(ctx, i.prom.Out())
	})
	return group.Wait()
}
