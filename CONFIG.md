grok_exporter Configuration
===========================

This page describes the `grok_exporter` configuration file.
The configuration is in YAML format. An example configuration can be found in [example/config.yml].
The path to the configuration file is passed as a command line parameter when starting `grok_exporter`:

```bash
grok_exporter -config ./example/config.yml
```

Overall Structure
-----------------

The `grok_exporter` configuration file consists of five main sections:

```yaml
global:
    # Config version
input:
    # How to read log lines (file or stdin).
grok:
    # Available Grok patterns.
metrics:
    # How to map Grok fields to Prometheus metrics.
server:
    # How to expose the metrics via HTTP(S).
```

The following shows the configuration options for each of these sections.

Global Section
--------------

The `global` section is as follows:

```yaml
global:
    config_version: 2
    retention_check_interval: 53s
```

The `config_version` specifies the version of the config file format. Specifying the `config_version` is mandatory, it has to be included in every configuration file. The current `config_version` is `2`.

The config file format is versioned independently of the `grok_exporter` program. When a new version of `grok_exporter` keeps using the same config file, the `config_version` will remain the same.

The following table shows which `grok_exporter` version uses which `config_version`:

| grok_exporter              | config_version           |
| -------------------------- | ------------------------ |
| ≤ 0.1.4                    | 1 _(see [CONFIG_v1.md])_ |
| 0.2.0, 0.2.1, 0.2.2, 0.2.3 | 2 _(current version)_    |

The `retention_check_interval` is the interval at which `grok_exporter` checks for expired metrics. By default, metrics don't expire so this is relevant only if `retention` is configured explicitly with a metric. The `retention_check_interval` is optional, the value defaults to `53s`. The default value is reasonable for production and should not be changed. This property is intended to be used in tests, where you might not want to wait 53 seconds until an expired metric is cleaned up. The format is described in [How to Configure Durations] below.

Input Section
-------------

We currently support two input types: `file` and `stdin`. The following two sections describe the `file` input type and the `stdin` input type:

### File Input Type

The configuration for the `file` input type is as follows:

```yaml
input:
    type: file
    path: /var/log/sample.log
    readall: false
    fail_on_missing_logfile: true
    poll_interval_seconds: 5 # should not be needed in most cases, see below
```

The `readall` flag defines if `grok_exporter` starts reading from the beginning or the end of the file.
True means we read the whole file, false means we start at the end of the file and read only new lines.
True is good for debugging, because we process all available log lines.
False is good for production, because we avoid to process lines multiple times when `grok_exporter` is restarted.
The default value for `readall` is `false`.

If `fail_on_missing_logfile` is true, `grok_exporter` will not start if the `path` is not found.
This is the default value, and it should be used in most cases because a missing logfile is likely a configuration error.
However, in some scenarios you might want `grok_exporter` to start successfully even if the logfile is not found,
because you know the file will be created later. In that case, set `fail_on_missing_logfile: false`.

On `poll_interval_seconds`: You probably don't need this. The internal implementation of `grok_exporter`'s
file input is based on the operating system's file system notification mechanism, which is `inotify` on Linux,
`kevent` on BSD (or macOS), and `ReadDirectoryChangesW` on Windows. These tools will inform `grok_exporter` as
soon as a new log line is written to the log file, and let `grok_exporter` sleep as long as the log file doesn't
change. There is no need for configuring a poll interval. However, there is one combination where the above
notifications don't work: If the logging application keeps the logfile open and the underlying file system is NTFS
(see [#17](https://github.com/fstab/grok_exporter/issues/17)). For this specific case you can configure a
`poll_interval_seconds`. This will disable file system notifications and instead check the log file periodically.
The `poll_interval_seconds` option was introduced with release 0.2.2.

### Stdin Input Type

The configuration for the `stdin` input type does not have any additional parameters:

```yaml
input:
    type: stdin
```

This is useful if you want to pipe log data to the `grok_exporter` command,
for example if you want to monitor the output of `journalctl`:

```bash
journalctl -f | grok_exporter -config config.yml
```

Note that `grok_exporter` terminates as soon as it finishes reading from `stdin`.
That means, if we run `cat sample.log | grok_exporter -config config.yml`,
the exporter will terminate as soon as `sample.log` is processed,
and we will not be able to access the result via HTTP(S) after that.
Always use a command that keeps the output open (like `tail -f`) when testing the `grok_exporter` with the `stdin` input.

Grok Section
------------

The `grok` section configures the available Grok patterns. An example configuration is as follows:

```yaml
grok:
    patterns_dir: ./logstash-patterns-core/patterns
    additional_patterns:
    - 'EXIM_MESSAGE [a-zA-Z ]*'
    - 'EXIM_SENDER_ADDRESS F=<%{EMAILADDRESS}>'
```

In most cases, we will have a directory containing the Grok pattern files. Grok's default pattern directory is included in the `grok_exporter` release. The path to this directory is configured with `patterns_dir`.

There are two ways to define additional Grok patterns:

1. Create a custom pattern file and store it in the `patterns_dir` directory.
2. Add pattern definitions directly to the `grok_exporter` configuration. This can be done via the `additional_patterns` configuration. It takes a list of pattern definitions. The pattern definitions have the same format as the lines in the Grok pattern files.

Grok patterns are simply key/value pairs: The key is the pattern name, and the value is a Grok macro defining a regular expression. There is a lot of documentation available on Grok patterns: The [logstash-patterns-core repository] contains [pre-defined patterns], the [Grok documentation] shows how patterns are defined, and there are online pattern builders available here: [http://grokdebug.herokuapp.com] and here: [http://grokconstructor.appspot.com].

At least one of `patterns_dir` or `additional_patterns` is required: If `patterns_dir` is missing all patterns must be defined directly in the `additional_patterns` config. If `additional_patterns` is missing all patterns must be defined in the `patterns_dir`.

Metrics Section
---------------

The metrics section contains a list of metric definitions, specifying how log lines are mapped to Prometheus metrics. Four metric types are supported:

* [Counter](#counter-metric-type)
* [Gauge](#gauge-metric-type)
* [Histogram](#histogram-metric-type)
* [Summary](#summary-metric-type)

To exemplify the different metrics configurations, we use the following example log lines:

```
30.07.2016 14:37:03 alice 1.5
30.07.2016 14:37:33 alice 2.5
30.07.2016 14:43:02 bob 2.5
30.07.2016 14:45:59 alice 2.5
```

Each line consists of a date, time, user, and a number. Using [Grok's default patterns], we can create a simple Grok expression matching these lines:

```grok
%{DATE} %{TIME} %{USER} %{NUMBER}
```

One of the main features of Prometheus is its multi-dimensional data model: A Prometheus metric can be further partitioned using different labels.

Labels are defined in two steps:

1. _Define Grok field names._ In Grok, each field, like `%{USER}`, can be given a name, like `%{USER:user}`. The name `user` can then be used in label templates.
2. _Define label templates._ Each metric type supports `labels`, which is a map of name/template pairs. The name will be used in Prometheus as the label name. The template is a [Go template] that may contain references to Grok fields, like `{{.user}}`.

Example: In order to define a label `user` for the example log lines above, use the following fragment:

```yaml
match: '%{DATE} %{TIME} %{USER:user} %{NUMBER}'
labels:
    user: '{{.user}}'
```

The `match` stores whatever matches the `%{USER}` pattern under the Grok field name `user`. The label defines a Prometheus label `user` with the value of the Grok field `user` as its content.

This simple example shows a one-to-one mapping of a Grok field to a Prometheus label. However, the label definition is pretty flexible: You can combine multiple Grok fields in one label, and you can define constant labels that don't use Grok fields at all.

### Expiring Old Labels

By default, metrics are kept forever. However, sometimes you might want metrics with old labels to expire. There are two ways to do this in `grok_exporter`:

#### `delete_labels`

As of version 0.2.2, `grok_exporter` supports `delete_match` and `delete_labels` configuration:

```yaml
delete_match: '%{DATE} %{TIME} %{USER:user} logged out'
delete_labels:
    user: '{{.user}}'
```

Without `delete_match` and `delete_labels`, all labels are kept forever (until `grok_exporter` is restarted). However, it might sometimes be desirable to explicitly remove metrics with specific labels. For example, if a service shuts down, it might be desirable to remove metrics labeled with that service name.

Using `delete_match` you can define a regular expression that will trigger removal of metrics. For example, `delete_match` could match a shutdown message in a log file.

Using `delete_labels` you can restrict which labels are deleted if a line matches `delete_match`. If no `delete_labels` are specified, all labels for the given metric are deleted. If `delete_labels` are specified, only those metrics are deleted where the label values are equal to the delete label values.

#### `retention`

As of version 0.2.3, `grok_exporter` supports `retention` configuration for metrics:

```yaml
metrics:
    - type: ...
      name: retention_example
      help: ...
      match: ...
      labels:
          ...
      retention: 2h30m
```

The example above means that if label values for the metrics named `retention_example` have not been observed for 2 hours and 30 minutes, the `retention_example` metrics with these label values will be removed.
For the format of the `retention` value, see [How to Configure Durations] below.
Note that `grok_exporter` checks the `retention` every 53 seconds by default, so it may take 53 seconds until the metric is actually removed after the retention time is reached, see `retention_check_interval` above.

### Counter Metric Type

The [counter metric] counts the number of matching log lines.

```yaml
metrics:
    - type: counter
      name: grok_example_lines_total
      help: Example counter metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER}'
      labels:
          user: '{{.user}}'
```

The configuration is as follows:
* `type` is `counter`.
* `name` is the name of the metric. Metric names are described in the [Prometheus data model documentation].
* `help` is a comment describing the metric.
* `match` is the Grok expression. See the [Grok documentation] for more info.
* `labels` is an optional map of name/template pairs, as described above.

Output for the example log lines above:

```
# HELP grok_example_lines_total Example counter metric with labels.
# TYPE grok_example_lines_total counter
grok_example_lines_total{user="alice"} 3
grok_example_lines_total{user="bob"} 1
```

### Gauge Metric Type

The [gauge metric] is used to monitor values that are logged with each matching log line.

```yaml
metrics:
    - type: gauge
      name: grok_example_values
      help: Example gauge metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      cumulative: false
      labels:
          user: '{{.user}}'
```

The configuration is as follows:
* `type` is `gauge`.
* `name`, `help`, `match`, and `labels` have the same meaning as for `counter` metrics.
* `value` is a [Go template] for the value to be monitored. The template must evaluate to a valid number. The template may use to Grok fields from the `match` patterns, like the label templates described above.
* `cumulative` is optional. By default, the last observed value is measured. With `cumulative: true`, the sum of all observed values is measured.

Output for the example log lines above::

```
# HELP grok_example_values Example gauge metric with labels.
# TYPE grok_example_values gauge
grok_example_values{user="alice"} 6.5
grok_example_values{user="bob"} 2.5
```

### Histogram Metric Type

Like `gauge` metrics, the [histogram metric] monitors values that are logged with each matching log line. However, instead of just summing up the values, histograms count the observed values in configurable buckets.

```yaml
    - type: histogram
      name: grok_example_values
      help: Example histogram metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      buckets: [1, 2, 3]
      labels:
          user: '{{.user}}'
```

The configuration is as follows:
* `type` is `histogram`.
* `name`, `help`, `match`, `labels`, and `value` have the same meaning as for `gauge` metrics.
* `buckets` configure the categories to be observed. In the example, we have 4 buckets: One for values < 1, one for values < 2, one for values < 3, and one for all values (i.e. < infinity). Buckets are optional. The default buckets are `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]`, which is useful for HTTP response times in seconds.

Output for the example log lines above::
```
# HELP grok_example_values Example histogram metric with labels.
# TYPE grok_example_values histogram
grok_example_values_bucket{user="alice",le="1"} 0
grok_example_values_bucket{user="alice",le="2"} 1
grok_example_values_bucket{user="alice",le="3"} 3
grok_example_values_bucket{user="alice",le="+Inf"} 3
grok_example_values_sum{user="alice"} 6.5
grok_example_values_count{user="alice"} 3
grok_example_values_bucket{user="bob",le="1"} 0
grok_example_values_bucket{user="bob",le="2"} 0
grok_example_values_bucket{user="bob",le="3"} 1
grok_example_values_bucket{user="bob",le="+Inf"} 1
grok_example_values_sum{user="bob"} 2.5
grok_example_values_count{user="bob"} 1
```

### Summary Metric Type

Like `gauge` and `histogram` metrics, the [summary metric] monitors values that are logged with each matching log line. Summaries measure configurable φ quantiles, like the median (φ=0.5) or the 95% quantile (φ=0.95). See [histograms and summaries] for more info.

```yaml
metrics:
   - type: summary
      name: grok_example_values
      help: Summary metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          user: '{{.user}}'
```

The configuration is as follows:
* `type` is `summary`.
* `name`, `help`, `match`, `labels`, and `value` have the same meaning as for `gauge` metrics.
* `quantiles` is a list of quantiles to be observed. `grok_exporter` does not provide exact values for the quantiles, but only estimations. For each quantile, you also specify an uncertainty that is tolerated for the estimation. In the example, we measure the median (0.5 quantile) with uncertainty 5%, the 90% quantile with uncertainty 1%, and the 99% quantile with uncertainty 0.1%. `quantiles` is optional, the default value is `{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}`.

Summaries represent a sliding time window of 10 minutes, i.e. if you observe a 0.5 quantile (median) of _x_, the value _x_ represents the median within the last 10 minutes. The time window is moved forward every 2 minutes.

Output for the example log lines above::
```
# HELP grok_example_values Example summary metric with labels.
# TYPE grok_example_values summary
grok_example_values{user="alice",quantile="0.5"} 2.5
grok_example_values{user="alice",quantile="0.9"} 2.5
grok_example_values{user="alice",quantile="0.99"} 2.5
grok_example_values_sum{user="alice"} 6.5
grok_example_values_count{user="alice"} 3
grok_example_values{user="bob",quantile="0.5"} 2.5
grok_example_values{user="bob",quantile="0.9"} 2.5
grok_example_values{user="bob",quantile="0.99"} 2.5
grok_example_values_sum{user="bob"} 2.5
grok_example_values_count{user="bob"} 1
```

Server Section
--------------

The server section configures the HTTP(S) server for exposing the metrics:

```yaml
server:
    protocol: https
    host: localhost
    port: 9144
    path: /metrics
    cert: /path/to/cert
    key: /path/to/key
```

* `protocol` can be `http` or `https`. Default is `http`.
* `host` can be a hostname or an IP address. If host is specified, `grok_exporter` will listen on the network interface with the given address. If host is omitted, `grok_exporter` will listen on all available network interfaces.
* `port` is the TCP port to be used. Default is `9144`.
* `path` is the path where the metrics are exposed. Default is `/metrics`, i.e. by default metrics will be exported on [http://localhost:9144/metrics].
* `cert` is the path to the SSL certificate file for protocol `https`. It is optional. If omitted, a hard-coded default certificate will be used.
* `key` is the path to the SSL key file for protocol `https`. It is optional. If omitted, a hard-coded default key will be used.

How to Configure Durations
--------------------------

`grok_exporter` uses the format from golang's [time.ParseDuration()] for configuring time intervals. Some examples are:

* `2h30m`: 2 hours and 30 minutes
* `100ms`: 100 milliseconds
* `1m30s`: 1 minute and 30 seconds
* `5m`: 5 minutes

[example/config.yml]: example/config.yml
[CONFIG_v1.md]: CONFIG_v1.md
[How to Configure Durations]: #how-to-configure-durations
[logstash-patterns-core repository]: https://github.com/logstash-plugins/logstash-patterns-core
[pre-defined patterns]: https://github.com/logstash-plugins/logstash-patterns-core/tree/master/patterns
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[http://grokdebug.herokuapp.com]: http://grokdebug.herokuapp.com
[http://grokconstructor.appspot.com]: http://grokconstructor.appspot.com
[Grok's default patterns]: https://github.com/logstash-plugins/logstash-patterns-core/blob/master/patterns/grok-patterns
[Go template]: https://golang.org/pkg/text/template/
[counter metric]: https://prometheus.io/docs/concepts/metric_types/#counter
[gauge metric]: https://prometheus.io/docs/concepts/metric_types/#gauge
[summary metric]: https://prometheus.io/docs/concepts/metric_types/#summary
[histogram metric]: https://prometheus.io/docs/concepts/metric_types/#histogram
[release]: https://github.com/fstab/grok_exporter/releases
[Prometheus metric types]: https://prometheus.io/docs/concepts/metric_types
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[histograms and summaries]: https://prometheus.io/docs/practices/histograms/
[time.ParseDuration()]: https://golang.org/pkg/time/#ParseDuration
[http://localhost:9144/metrics]: http://localhost:9144/metrics
