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

The `grok_exporter` configuration file consists of four main sections:

```yaml
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
```

The `readall` flag defines if `grok_exporter` starts reading from the beginning or the end of the file.
True means we read the whole file, false means we start at the end of the file and read only new lines.
True is good for debugging, because we process all available log lines.
False is good for production, because we avoid to process lines multiple times when `grok_exporter` is restarted.
The default value for `readall` is `false`.

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

In most cases, we will have a directory containing the Grok pattern files. Grok's default pattern directory is included in the `grok_exporter` release. The path to that directory is configured with `patterns_dir`.

There are two ways to define additional Grok patterns:

1. Create a custom pattern file and store it in the `patterns_dir` directory.
2. Add pattern definitions directly to the `grok_exporter` configuration. This can be done via the `additional_patterns` configuration. It takes a list of pattern definitions. The pattern definitions have the same format as the lines in the Grok pattern files.

Grok patterns are simply key/value pairs: The key is the pattern name, and the value is a Grok macro defining a regular expression. There is a lot of documentation available on Grok patterns: The [logstash-patterns-core repository] contains [pre-defined patterns], the [Grok documentation] shows how patterns are defined, and there are online pattern builders available here: [http://grokdebug.herokuapp.com] and here: [http://grokconstructor.appspot.com].

At least one of `patterns_dir` or `additional_patterns` is required: If `patterns_dir` is missing all patterns must be defined directly in the `additional_patterns` config. If `additional_patterns` is missing all patterns must be defined in the `patterns_dir`.

Metrics Section
---------------

The metrics section contains a list of metrics. These metrics define how Grok fields are mapped to Prometheus metrics.

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

One of the main features of Prometheus is its multi-dimensional data model: A Prometheus metric can be further partitioned using different labels. In Grok, each field, like `%{USER}`, can be given a name, like `%{USER:user}`. With `grok_exporter`, we can use Grok field names as Prometheus labels.

The resulting Grok expression for the log lines above would be as follows:

```grok
%{DATE} %{TIME} %{USER:user} %{NUMBER}
```

The Grok field `user` can now be used as a Prometheus label, as shown in the sections below.

### Counter Metric Type

The [counter metric] counts the number of matching log lines.

```yaml
metrics:
    - type: counter
      name: grok_example_lines_total
      help: Example counter metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER}'
      labels:
          - grok_field_name: user
            prometheus_label: user
```

The configuration is as follows:
* `type` is `counter`.
* `name` is the name of the metric. Metric names are described in the [Prometheus data model documentation].
* `help` is a comment describing the metric.
* `match` is the Grok expression. See the [Grok documentation] for more info.
* `labels` is optional and can be used to partition the metric by Grok fields. `labels` contains a list of `grok_field_name`/`prometheus_label` pairs. In the example `match` pattern, we used `%{USER:user}` to define that the match for `%{USER}` will have the Grok field name `user`. In `labels`, we configured that the `grok_field_name: user` is mapped to the the `prometheus_label: user`, so the Prometheus label has the same name as the Grok field. However, it is common to use different names for the Grok field and the Prometheus label, because Prometheus has other naming conventions than Grok. The [Prometheus data model documentation] has more info on Prometheus label names.

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
      value: val
      cumulative: false
      labels:
          - grok_field_name: user
            prometheus_label: user
```

The configuration is as follows:
* `type` is `gauge`.
* `name`, `help`, `match`, and `labels` have the same meaning as for `counter` metrics.
* `value` is the Grok field to be monitored. In the example `match` pattern, we used `%{NUMBER:val}` to define that the match for `%{NUMBER}` will have the name `val`. We then use `val` as the `value` to be monitored. You must make sure that the Grok field used as `value` always matches a valid number.
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
      value: val
      buckets: [1, 2, 3]
      labels:
          - grok_field_name: user
            prometheus_label: user
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
      value: val
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          - grok_field_name: user
            prometheus_label: user
```

The configuration is as follows:
* `type` is `summary`.
* `name`, `help`, `match`, `labels`, and `value` have the same meaning as for `gauge` metrics.
* `quantiles` is a list of quantiles to be observed. `grok_exporter` does not provide exact values for the quantiles, but only estimations. For each quantile, you also specify an uncertainty that is tolerated for the estimation. In the example, we measure the median (0.5 quantile) with uncertainty 5%, the 90% quantile with uncertainty 1%, and the 99% quantile with uncertainty 0.1%. `quantiles` is optional, the default value is `{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}`.

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
    cert: /path/to/cert
    key: /path/to/key
```

* `protocol` can be `http` or `https`. Default is `http`.
* `host` can be a hostname or an IP address. If host is specified, `grok_exporter` will listen on the network interface with the given address. If host is omitted, `grok_exporter` will listen on all available network interfaces.
* `port` is the TCP port to be used. Default is `9144`.
* `cert` is the path to the SSL certificate file for protocol `https`. It is optional. If omitted, a hard-coded default certificate will be used.
* `key` is the path to the SSL key file for protocol `https`. It is optional. If omitted, a hard-coded default key will be used.

[example/config.yml]: example/config.yml
[logstash-patterns-core repository]: https://github.com/logstash-plugins/logstash-patterns-core
[pre-defined patterns]: https://github.com/logstash-plugins/logstash-patterns-core/tree/master/patterns
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[http://grokdebug.herokuapp.com]: http://grokdebug.herokuapp.com
[http://grokconstructor.appspot.com]: http://grokconstructor.appspot.com
[Grok's default patterns]: https://github.com/logstash-plugins/logstash-patterns-core/blob/master/patterns/grok-patterns 
[counter metric]: https://prometheus.io/docs/concepts/metric_types/#counter
[gauge metric]: https://prometheus.io/docs/concepts/metric_types/#gauge
[summary metric]: https://prometheus.io/docs/concepts/metric_types/#summary
[histogram metric]: https://prometheus.io/docs/concepts/metric_types/#histogram
[release]: https://github.com/fstab/grok_exporter/releases
[Prometheus metric types]: https://prometheus.io/docs/concepts/metric_types
[Prometheus data model documentation]: https://prometheus.io/docs/concepts/data_model
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[histograms and summaries]: https://prometheus.io/docs/practices/histograms/
