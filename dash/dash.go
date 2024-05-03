package dash

import (
	"context"
	"fmt"
	"time"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/prometheus/prometheus/promql"
	"golang.org/x/sync/errgroup"
)

const redrawInterval = 250 * time.Millisecond

type Dash struct {
	terminal  *tcell.Terminal
	container *container.Container
	chart     *linechart.LineChart
}

func New() (_ *Dash, err error) {
	t, err := tcell.New()
	if err != nil {
		return nil, fmt.Errorf("error creating terminal: %w", err)
	}
	defer func() {
		if err != nil {
			t.Close()
		}
	}()

	lc, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorCyan)),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating line chart: %w", err)
	}

	c, err := container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("PRESS Q TO QUIT"),
		container.PlaceWidget(lc),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating container: %w", err)
	}

	return &Dash{
		terminal:  t,
		container: c,
		chart:     lc,
	}, nil
}

func (d *Dash) Run(ctx context.Context, in <-chan promql.Matrix) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
	}

	group.Go(func() error {
		return termdash.Run(
			ctx,
			d.terminal,
			d.container,
			termdash.KeyboardSubscriber(quitter),
			termdash.RedrawInterval(redrawInterval),
		)
	})
	group.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case data, ok := <-in:
				if !ok {
					return nil
				}
				err := d.drawLineChart(data)
				if err != nil {
					return err
				}
			}
		}
	})

	return group.Wait()
}

func (d *Dash) drawLineChart(matrix promql.Matrix) error {
	series := matrix[0] // TODO handle multiple series
	input := make([]float64, len(series.Floats))
	for i, f := range series.Floats {
		input[i] = f.F
	}
	err := d.chart.Series(
		"first",
		input,
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	if err != nil {
		return fmt.Errorf("error drawing line chart: %w", err)
	}
	return nil
}
