package dash

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/lovromazgon/impromptu/opt"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/gauge"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/prometheus/prometheus/promql"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const redrawInterval = 250 * time.Millisecond

type Dash struct {
	queryRange    time.Duration
	queryInterval time.Duration

	chart      *linechart.LineChart
	timestamps []int64

	// TODO vacuum old data
	data map[int64]float64
	m    sync.Mutex
}

func New(options opt.Options) (_ *Dash, err error) {
	lc, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorCyan)),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating line chart: %w", err)
	}

	count := int(options.QueryRange / options.QueryInterval)
	timestamps := make([]int64, count)
	series := make([]float64, count)
	now := time.Now().Truncate(options.ScrapeInterval)
	for i := range timestamps {
		timestamps[i] = now.Add(-options.QueryRange + time.Duration(i)*options.QueryInterval).UnixMilli()
	}

	err = lc.Series("first", series)
	if err != nil {
		return nil, fmt.Errorf("error setting initial series: %w", err)
	}

	return &Dash{
		queryRange:    options.QueryRange,
		queryInterval: options.QueryInterval,

		chart:      lc,
		timestamps: timestamps,

		data: make(map[int64]float64),
	}, nil
}

func (d *Dash) Run(ctx context.Context, in <-chan promql.Matrix) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	terminal, err := tcell.New()
	if err != nil {
		return fmt.Errorf("error creating terminal: %w", err)
	}
	defer terminal.Close()

	c, err := container.New(
		terminal,
		container.ID("root"),
		container.Border(linestyle.Light),
		container.BorderTitle("PRESS Q TO QUIT"),
		container.PlaceWidget(d.chart),
	)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}

	group, ctx := errgroup.WithContext(ctx)

	// ---- remove this once the initial Prometheus delay is fixed ----
	err = d.setupInitialDelayGauge(ctx, c, group)
	if err != nil {
		return err
	}
	// ----------------------------------------------------------------

	group.Go(func() error {
		return d.drawLineChart(ctx)
	})
	group.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case data, ok := <-in:
				if !ok {
					return nil
				}
				d.handlePromMatrix(data)
			}
		}
	})

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
	}
	err = termdash.Run(
		ctx,
		terminal,
		c,
		termdash.KeyboardSubscriber(quitter),
		termdash.RedrawInterval(redrawInterval),
	)
	cancel()
	return errors.Join(err, group.Wait())
}

func (d *Dash) handlePromMatrix(matrix promql.Matrix) {
	d.m.Lock()
	defer d.m.Unlock()

	series := matrix[0] // TODO handle multiple series
	for _, f := range series.Floats {
		t := time.UnixMilli(f.T).Truncate(time.Second).UnixMilli()
		d.data[t] = f.F
	}
}

func (d *Dash) drawLineChart(ctx context.Context) error {
	rateLimit := rate.NewLimiter(rate.Every(d.queryInterval), 1)
	series := make([]float64, len(d.timestamps))
	for {
		err := rateLimit.Wait(ctx)
		if err != nil {
			return err
		}

		moveIndex := d.updateTimestamps()
		copy(series, series[moveIndex:])
		for i := len(series) - moveIndex; i < len(series); i++ {
			series[i] = 0
		}

		d.m.Lock()
		for t, f := range d.data {
			index, ok := slices.BinarySearch(d.timestamps, t)
			if !ok {
				continue
			}
			series[index] = f
		}
		d.m.Unlock()

		err = d.chart.Series(
			"first",
			series,
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
			linechart.SeriesXLabels(d.xLabels()),
		)
		if err != nil {
			return fmt.Errorf("error drawing line chart: %w", err)
		}
	}
}

func (d *Dash) updateTimestamps() int {
	now := time.Now().Truncate(time.Second)
	start := now.Add(-d.queryRange).UnixMilli()
	startIndex := len(d.timestamps)
	for i := range d.timestamps {
		if d.timestamps[i] == start {
			startIndex = i
			break
		}
	}
	if startIndex < len(d.timestamps) {
		copy(d.timestamps, d.timestamps[startIndex:])
	}
	for i := len(d.timestamps) - startIndex; i < len(d.timestamps); i++ {
		d.timestamps[i] = start + (int64(i) * d.queryInterval.Milliseconds())
	}
	return startIndex
}

func (d *Dash) xLabels() map[int]string {
	labels := make(map[int]string, len(d.timestamps))
	for i, t := range d.timestamps {
		// TODO cache labels
		labels[i] = time.UnixMilli(t).Format("15:04:05")
	}
	return labels
}

// setupInitialDelayGauge displays a gauge to inform the user about the initial
// scrape delay.
func (d *Dash) setupInitialDelayGauge(ctx context.Context, c *container.Container, group *errgroup.Group) error {
	const initialDelay = 5 // Prometheus has a hardcoded 5-second delay before it starts the first scrape
	initialGaugeText := "%d - Please wait for Prometheus to start scraping the target..."
	initialGauge, err := gauge.New(
		gauge.Height(3),
		gauge.TextLabel(fmt.Sprintf(initialGaugeText, initialDelay)),
		gauge.Color(cell.ColorGreen),
		gauge.FilledTextColor(cell.ColorBlack),
		gauge.EmptyTextColor(cell.ColorGreen),
		gauge.HideTextProgress(),
		gauge.Border(linestyle.Light),
	)
	if err != nil {
		return fmt.Errorf("error creating initial gauge: %w", err)
	}
	err = initialGauge.Absolute(0, initialDelay)
	if err != nil {
		return fmt.Errorf("error setting initial gauge: %w", err)
	}
	err = c.Update(
		"root",
		container.Border(linestyle.Light),
		container.BorderTitle("PRESS Q TO QUIT"),
		container.PlaceWidget(initialGauge),
	)
	if err != nil {
		return fmt.Errorf("error updating container: %w", err)
	}

	group.Go(func() error {
		for i := 0; i < initialDelay; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				err := initialGauge.Absolute(
					i+1, initialDelay,
					gauge.TextLabel(fmt.Sprintf(initialGaugeText, initialDelay-i-1)),
				)
				if err != nil {
					return fmt.Errorf("error updating gauge: %w", err)
				}
			}
		}
		err := c.Update(
			"root",
			container.Border(linestyle.Light),
			container.BorderTitle("PRESS Q TO QUIT"),
			container.PlaceWidget(d.chart),
		)
		if err != nil {
			return fmt.Errorf("error updating container: %w", err)
		}
		return nil
	})
	return nil
}
