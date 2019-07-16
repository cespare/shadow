# Shadow

Shadow is a small HTTP API to expose your Graphite metrics in a monitorable HTTP
format.

Shadow is inspired by [Umpire](https://github.com/heroku/umpire).

## Installation

Build from source using Go 1.12+:

    cd $(mktemp -d); go mod init tmp; go get github.com/cespare/shadow

(Control where the binary is placed using the GOBIN environment variable.)

## Usage

Edit the configuration file, then start it up:

    $ shadow -conf path/to/conf.toml

Shadow responds to queries at `/check`. The check properties are given in
query-string parameters.

* `metric`: The graphite metric key. Can be a query that returns multiple keys
  (e.g., using `*`, `[...]`, or `{...}`).
* `range`: How far back in the past to consider. The syntax is Go's
  `time.Duration` (see the
  [`time.ParseDuration`](http://golang.org/pkg/time/#ParseDuration)
  documentation for the details). Examples: `30s`, `5m`, `1h`.
* `limit`: A comma-separated list of comparisons between an absolute value and
  an aggregator term. The possible comparators are `<`, `<=`, `=`, `>=`, and
  `>`. The possible aggregator terms are `avg`, `min`, `max`, and `sum`.
  Example: `limit=min>500,avg>1000,avg<2000`.
* `group_limit`: This is like `limit`, except the possible aggregators are
  `count` and `fraction`. `group_limit` must be given when the target metric
  returns multiple values. This defines the limits on the number/fraction of
  successful targets needed for a successful check. Example:
  `group_limit=count>5`. There are two aliases, `any` and `all`, which can be
  used instead (e.g., `group_limit=all`).
* `include_empty_targets`: This parameter controls whether Shadow considers
  targets that come back without any non-null datapoints.

Shadow has a health check that lives at `/healthz`. This also checks that
Graphite is up as part of its health check.

## Web UI

Shadow query strings can get somewhat hard to read, especially with all the url
character escaping and when you have complex Graphite queries. So, Shadow
includes a little web page that helps you construct the query strings. Just go
to the root URL (for example, `http://localhost:2050`). (Note that for Shadow to
find its HTML/CSS/JS assets, it must be run from the repository root.)

![screenshot](/screenshot.png)

## Examples

Suppose you log your web server's requests at `web-1.requests.{count,rate}`. You
can make sure your mean qps doesn't fall below 300 for any 5-minute window:

    /check?metric=web-1.requests.rate&range=5m&limit=avg>300

Perhaps you also want to know if the load spikes above 2000 so you can spin up
more workers:

    /check?metric=web-1.requests.rate&range=5m&limit=avg>300,avg<2000

You have multiple servers, and you want to ensure that none of them have
out-of-whack qps:

    /check?metric=web-*.requests.rate&range=5m&limit=avg>300,avg<2000&group_limit=all

You're using [gost](https://github.com/cespare/gost) and you want to alert when
one of your servers is overloaded:

    /check?metric=backend-*.gost.os_stats.load_avg_15.gauge&range=1m&limit=max<0.7&group_limit=all

You have a massive HDFS cluster and you want to know when more than 10% of the
machines are running low on disk space:

    /check?metric=hdfs-*.gost.os_stats.disk_usage.root_volume.gauge&range=5m&limit=max<0.9&group_limit=fraction>0.9

## Advantages over Umpire

* Handy web UI for constructing queries
* Easier to deploy (Go vs. Ruby)
* More thorough error messages
* Supports group limits
* Richer bounding functionality
