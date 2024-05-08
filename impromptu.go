package impromptu

import (
	"context"
	"errors"
	"fmt"

	"github.com/lovromazgon/impromptu/dash"
	"github.com/lovromazgon/impromptu/opt"
	"github.com/lovromazgon/impromptu/prom"
	"golang.org/x/sync/errgroup"
)

// rate(conduit_pipeline_execution_duration_seconds_count[5s])

type Impromptu struct {
	prom *prom.Prom
	dash *dash.Dash
}

func New(options opt.Options) (*Impromptu, error) {
	p, err := prom.New(options)
	if err != nil {
		return nil, fmt.Errorf("error creating prom: %w", err)
	}

	d, err := dash.New(options)
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

	err := group.Wait()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
