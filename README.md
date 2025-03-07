<p align="center">
    <picture>
        <source media="(prefers-color-scheme: dark)" srcset="https://github.com/lovromazgon/impromptu/blob/main/logo/impromptu-white.png">
        <source media="(prefers-color-scheme: light)" srcset="https://github.com/lovromazgon/impromptu/blob/main/logo/impromptu-black.png">
        <img alt="Impromptu logo" width="600" src="https://github.com/lovromazgon/impromptu/blob/main/logo/impromptu-black.png">
    </picture>
</p>

[![License](https://img.shields.io/github/license/lovromazgon/impromptu)](https://github.com/ConduitIO/conduit/blob/main/LICENSE)
[![Test](https://github.com/lovromazgon/impromptu/actions/workflows/test.yml/badge.svg)](https://github.com/lovromazgon/impromptu/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/lovromazgon/impromptu)](https://goreportcard.com/report/github.com/lovromazgon/impromptu)

Impromptu is a tool that scrapes metrics from a Prometheus endpoint and
continuously executes a user-provided PromQL query to visualize the metrics
in the CLI.

You can use Impromptu to get an insight into the metrics of a service without
deploying Prometheus and Grafana, mostly during development and testing.

https://github.com/lovromazgon/impromptu/assets/8320753/088736b7-ac20-479e-b53c-fe98118f8136

## Installation

Install using homebrew:

```sh
brew install lovromazgon/tap/impromptu
```

Or download the binary manually from the [latest release](https://github.com/lovromazgon/impromptu/releases/latest).

> [!NOTE]
> When downloading Impromptu manually on MacOS you will get a warning about a safety issue.
> That's because Impromptu is currently not a signed binary, you have to do some
> [extra steps](https://support.apple.com/en-us/102445#openanyway) to make it run.

Once you have downloaded impromptu, you can try it out using this runnable example:

```sh
impromptu -t http://demo.do.prometheus.io:9100/metrics -q "rate(node_cpu_seconds_total{mode=\"idle\"}[5s])" -r 1m
```

## Usage

```
Usage:
    impromptu -t URL -q PROMQL_QUERY [-i DURATION] [-r DURATION]

Options:
    -t, --target-url URL               Fetch metrics from the specified URL
    -q, --query-string PROMQL_QUERY    Query to execute against the metrics
    -i, --query-interval DURATION      Interval to fetch metrics at [default: 1s]
    -r, --query-range DURATION         Range of the query [default: 5m]
    -v, --version                      Print version information

URL represents an endpoint that serves Prometheus metrics in text format.

PROMQL_QUERY is a Prometheus query language expression that returns a single
time series. The query is executed every second and the result is displayed
in a terminal chart. The query should return a single time series, e.g. a
rate or a sum of a counter. The query interval and range can be adjusted with
the -i and -r flags.

DURATION is a time duration string that can be parsed by Go's time.ParseDuration
function. It represents a time interval, e.g. "5m" for 5 minutes, "1h" for 1
hour, "30s" for 30 seconds, "1h 2m 3s" for 1 hour, 2 minutes and 3 seconds etc.

Example:
    $ impromptu -t http://demo.do.prometheus.io:9100/metrics -q "rate(node_cpu_seconds_total{mode=\"idle\"}[5s])" -r 1m
```

## Roadmap

- [X] Remove initial 5 second delay (PR: https://github.com/prometheus/prometheus/pull/14073)
- [ ] Support for multiple time series
- [ ] Support for other charts (donut, bar chart)
- [ ] Display legend
