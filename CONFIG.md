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

Grok patterns are key/value pairs: The key is the pattern name, and the value is a Grok macro defining a regular expression.
There is a lot of documentation available on Grok patterns: The [logstash-patterns-core repository] contains [pre-defined patterns],
the [Grok documentation] shows how patterns are defined, and there are online pattern builders available
here: [http://grokdebug.herokuapp.com] and here: [http://grokconstructor.appspot.com].

In most cases, we will have a directory containing all our pattern files.
This directory can be configured with `patterns_dir`. All files in this directory must be valid pattern definition files.
Examples of these files can be found in Grok's [pre-defined patterns].

The `additional_patterns` configuration defines a list of additional Grok patterns.
This is convenient to quickly add some patterns without the need to create new files in `patterns_dir`.
The lines defined in the `patterns` list have the same format as the lines in the files in `patterns_dir`.

`patterns_dir` and `additional_patterns` are both optional:
If `patterns_dir` is missing all patterns must be defined directly in the `additional_patterns` config.
If `additional_patterns` is missing all patterns must be defined in the `patterns_dir`.

Metrics Section
---------------

The metrics section contains a list of metrics.
These metrics define how Grok fields are mapped to Prometheus metrics:

```yaml
metrics:
    - type: counter
      name: exim_rejected_rcpt_total
      help: Total number of rejected recipients, partitioned by error message.
      match: '%{EXIM_DATE} %{EXIM_REMOTE_HOST} F=<%{EMAILADDRESS}> rejected RCPT <%{EMAILADDRESS}>: %{EXIM_MESSAGE:message}'
      labels:
          - grok_field_name: message
            prometheus_label: error_message
```

Each metric has a `type`, `name`, `help`, `match`, and `labels`.
Apart from that, there can be additional parameters depending on the metric type.
We describe the general metric configuration here, and provide additional info on specific metric types in the sections below.

* `type` corresponds to the [Prometheus metric type]. As of now, we only support `counter`.
* `name` is the name of the metric. Metric names are described in the [Prometheus data model documentation].
* `help` will be included as a comment when the metric is exposed via HTTP(S).
* `match` is the Grok expression. See the [Grok documentation] for more info.
* `labels` is optional and can be used to partition the metric by Grok fields.
  `labels` contains a list of `grok_field_name`/`prometheus_label` pairs.
  The `grok_field_name` must be a field name that is used in the `match`.
  For example, if `match` is `%{NUMBER:duration} %{IP:client}`, the names `duration` and `client` may be used as Grok field names.
  The `prometheus_label` defines how the Prometheus label will be called.
  It is common to use different names for the Grok field and the Prometheus label,
  because Prometheus has other naming conventions than Grok.
  The [Prometheus data model documentation] has more info on Prometheus label names.

### Counter Metric Type

The counter metric is incremented whenever a log line matches. There are no additional configuration parameters for counter metrics.

### Gauge Metric Type

_Not implemented yet._

Server Section
--------------

The server section configures the HTTP(S) server for exposing the metrics:

```yaml
server:
    protocol: https
    port: 9144
    cert: /path/to/cert
    key: /path/to/key
```

* `protocol` can be `http` or `https`. Default is `http`.
* `port` is the TCP port to be used. Default is `9144`.
* `cert` is the path to the SSL certificate file for protocol `https`. It is optional. If omitted, a hard-coded default certificate will be used.
* `key` is the path to the SSL key file for protocol `https`. It is optional. If omitted, a hard-coded default key will be used.

[example/config.yml]: example/config.yml
[logstash-patterns-core repository]: https://github.com/logstash-plugins/logstash-patterns-core
[pre-defined patterns]: https://github.com/logstash-plugins/logstash-patterns-core/tree/master/patterns
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[http://grokdebug.herokuapp.com]: http://grokdebug.herokuapp.com
[http://grokconstructor.appspot.com]: http://grokconstructor.appspot.com
[Prometheus metric type]: https://prometheus.io/docs/concepts/metric_types
[Prometheus data model documentation]: https://prometheus.io/docs/concepts/data_model
[Grok documentation]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html