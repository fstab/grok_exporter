[![Build Status](https://travis-ci.org/fstab/grok_exporter.svg?branch=master)](https://travis-ci.org/fstab/grok_exporter) [![Build status](https://ci.appveyor.com/api/projects/status/d8aq0pa3yfoapd69?svg=true)](https://ci.appveyor.com/project/fstab/grok-exporter) [![Coverage Status](https://coveralls.io/repos/github/fstab/grok_exporter/badge.svg?branch=master)](https://coveralls.io/github/fstab/grok_exporter?branch=master)

grok_exporter
=============

Export [Prometheus] metrics from arbitrary unstructured log data.

About Grok
----------

[Grok] is a tool to parse crappy unstructured log data into something structured and queryable. Grok is heavily used in [Logstash] to provide log data as input for [ElasticSearch].

Grok ships with about 120 predefined patterns for syslog logs, apache and other webserver logs, mysql logs, etc. It is easy to extend Grok with custom patterns.

The `grok_exporter` aims at porting Grok from the [ELK stack] to [Prometheus] monitoring. The goal is to use Grok patterns for extracting Prometheus metrics from arbitrary log files.

How to run the example
----------------------

Download `grok_exporter-$ARCH.zip` for your operating system from the [releases] page, extract the archive, `cd grok_exporter-$ARCH`, then run

```bash
./grok_exporter -config ./example/config.yml
```

The example log file `exim-rejected-RCPT-examples.log` contains log messages from the [Exim] mail server. The configuration in `config.yml` counts the total number of rejected recipients, partitioned by error message.

The exporter provides the metrics on [http://localhost:9144/metrics]:

![screenshot.png]

Configuration
-------------

Example configuration:

```yaml
global:
    config_version: 2
input:
    type: file
    path: ./example/example.log
    readall: true
grok:
    patterns_dir: ./logstash-patterns-core/patterns
metrics:
    - type: counter
      name: grok_example_lines_total
      help: Counter metric example with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER}'
      labels:
          user: '{{.user}}'
server:
    port: 9144
```

[CONFIG.md] describes the `grok_exporter` configuration file and shows how to define Grok patterns, Prometheus metrics, and labels.  It also details how to configure file, stdin, and webhook inputs.

Status
------

Operating system support:

* Linux 64 Bit: [Supported](https://travis-ci.org/fstab/grok_exporter)
* Windows 64 Bit: [Supported](https://ci.appveyor.com/project/fstab/grok-exporter)
* mac OS 64 Bit: [Supported](https://travis-ci.org/fstab/grok_exporter)

Grok pattern support:

* We are able to compile all of Grok's default patterns on [github.com/logstash-plugins/logstash-patterns-core](https://github.com/logstash-plugins/logstash-patterns-core/tree/818b7aa60d3c2fea008ea673dbbc49179c6df2c8/patterns).

Prometheus support:

* [Counter] metrics: [Supported](CONFIG.md#metrics-section)
* [Gauge] metrics: [Supported](CONFIG.md#metrics-section)
* [Histogram] metrics: [Supported](CONFIG.md#metrics-section)
* [Summary] metrics: [Supported](CONFIG.md#metrics-section)

How to build from source
-----------------------

**Note: `grok_exporter` is currently refactored to support multiple logfiles, see [#5](https://github.com/fstab/grok_exporter/issues/5). During transition, the `master` branch is unstable. For a stable version, please get the latest release tag.**

In order to compile `grok_exporter` from source, you need

* [Go] installed and `$GOPATH` set.
* [gcc] installed for `cgo`. On Ubuntu, use `apt-get install build-essential`.
* Header files for the [Oniguruma] regular expression library, see below.

**Installing the Oniguruma library on OS X**

```bash
brew install oniguruma
```

**Installing the Oniguruma library on Ubuntu Linux**

```bash
sudo apt-get install libonig-dev
```

**Installing the Oniguruma library from source**

```bash
wget https://github.com/kkos/oniguruma/releases/download/v6.7.0/onig-6.7.0.tar.gz
tar xfz onig-6.7.0.tar.gz
cd onig-6.7.0.tar.gz && ./configure && make && make install
```

**Installing grok_exporter**

With Go 1.11, you can use the new Modules (no need for `go get`). If you are working inside the GOPATH, you need to `export GO111MODULE=on` to enable Go 1.11 Modules.

```bash
git clone https://github.com/fstab/grok_exporter
cd grok_exporter
git submodule update --init --recursive
go install .
```

With Go 1.10, use `go get`:

```bash
go get github.com/fstab/grok_exporter
cd $GOPATH/src/github.com/fstab/grok_exporter
git submodule update --init --recursive
```

The resulting `grok_exporter` binary will be dynamically linked to the Oniguruma library, i.e. it needs the Oniguruma library to run. The [releases] are statically linked with Oniguruma, i.e. the releases don't require Oniguruma as a run-time dependency. The releases are built with `release.sh`.

More Documentation
------------------

User documentation is included in the [GitHub repository]:

* [CONFIG.md]: Specification of the config file.
* [BUILTIN.md]: Definition of metrics provided out-of-the-box.

Developer notes are available on the [GitHub Wiki pages]:

* [tailer (tail -f)](https://github.com/fstab/grok_exporter/wiki/tailer-(tail-%E2%80%90f))
* [About the Regular Expression Library](https://github.com/fstab/grok_exporter/wiki/About-the-Regular-Expression-Library)

External documentation:

* Extracting Prometheus Metrics from Application Logs - [https://labs.consol.de/...](https://labs.consol.de/monitoring/2016/07/31/Prometheus-Logfile-Monitoring.html)
* Counting Errors with Prometheus - [https://labs.consol.de/...](https://labs.consol.de/monitoring/2016/08/13/counting-errors-with-prometheus.html)
* **\[Video\]** Lightning talk on grok_exporter - [https://www.youtube.com/...](https://www.youtube.com/watch?v=jFX8BVT4V_g)

Contact
-------

* For feature requests, bugs reports, etc: Please open a GitHub issue.
* For bug fixes, contributions, etc: Create a pull request.
* Questions? Contact me at fabian@fstab.de.

Related Projects
----------------

Google's [mtail] goes in a similar direction. It uses its own pattern definition language, so it will not work out-of-the-box with existing Grok patterns. However, `mtail`'s [RE2] regular expressions are probably [more CPU efficient] than Grok's [Oniguruma] patterns. `mtail` reads logfiles using the [fsnotify] library, which [might be an obstacle] on operating systems other than Linux.

License
-------

Licensed under the Apache License, Version 2.0.
You may obtain a copy of the License at [http://www.apache.org/licenses/LICENSE-2.0].

[Prometheus]: https://prometheus.io/
[Grok]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[Logstash]: https://www.elastic.co/products/logstash
[ElasticSearch]: https://www.elastic.co/
[ELK stack]: https://www.elastic.co/webinars/introduction-elk-stack
[Exim]: http://www.exim.org/
[Go]: https://golang.org/
[Oniguruma]: https://github.com/kkos/oniguruma
[screenshot.png]: screenshot.png
[releases]: https://github.com/fstab/grok_exporter/releases
[http://localhost:9144/metrics]: http://localhost:9144/metrics
[CONFIG.md]: CONFIG.md
[BUILTIN.md]: BUILTIN.md
[regexp]: https://golang.org/pkg/regexp
[RE2]: https://github.com/google/re2/wiki/Syntax
[mtail]: https://github.com/google/mtail
[regexp2]: https://github.com/dlclark/regexp2
[pcre]: https://github.com/glenn-brown/golang-pkg-pcre
[libpcre]: http://www.pcre.org
[rubex]: https://github.com/moovweb/rubex
[http://www.apache.org/licenses/LICENSE-2.0]: http://www.apache.org/licenses/LICENSE-2.0
[more CPU efficient]: https://github.com/fstab/grok_exporter/wiki/About-the-Regular-Expression-Library
[fsnotify]: https://github.com/fsnotify/fsnotify
[might be an obstacle]: https://github.com/fstab/grok_exporter/wiki/tailer-(tail-%E2%80%90f)
[GitHub Wiki pages]: https://github.com/fstab/grok_exporter/wiki
[GitHub repository]: https://github.com/fstab/grok_exporter
[Counter]: https://prometheus.io/docs/concepts/metric_types/#counter
[Gauge]: https://prometheus.io/docs/concepts/metric_types/#gauge
[Histogram]: https://prometheus.io/docs/concepts/metric_types/#histogram
[Summary]: https://prometheus.io/docs/concepts/metric_types/#summary
[https://groups.google.com/forum/#!forum/grok_exporter-users]: https://groups.google.com/forum/#!forum/grok_exporter-users
