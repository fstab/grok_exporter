grok_exporter
=============

Export [Prometheus] metrics from arbitrary unstructured log data.

About Grok
----------

[Grok] is tool to parse crappy unstructured log data into something structured and queryable.
Grok is heavily used in [Logstash] to provide log data as input for [ElasticSearch].

Grok ships with about 120 predefined patterns for syslog logs, apache and other webserver logs, mysql logs, etc.
It is easy to extend Grok with custom patterns.

The `grok_exporter` aims at porting Grok from the [ELK stack] to [Prometheus] monitoring.
The goal is to use Grok patterns for extracting Prometheus metrics from arbitrary log files.

How to run the example
----------------------

Download `grok_exporter-$ARCH.zip` for your operating system from the [releases] page, extract the archive, `cd grok_exporter-$ARCH`, then run

```bash
grok_exporter -config ./example/config.yml
```

The example directory contains `exim-rejected-RCPT-examples.log`, which is an example log file with sample log messages from the [Exim] mail server.
The configuration in `config.yml` counts the total number of rejected recipients, partitioned by error message.

The exporter provides the metrics on [http://localhost:9144/metrics]:

![screenshot.png]

Status
------

`grok_exporter` is still in alpha phase. We plan to release the first beta before the [PromCon 2016](https://promcon.io).

Operating system support:

* Linux 64 Bit: Supported [![Build Status](https://travis-ci.org/fstab/grok_exporter.svg?branch=master)](https://travis-ci.org/fstab/grok_exporter)
* Windows 64 Bit: Supported [![Build status](https://ci.appveyor.com/api/projects/status/d8aq0pa3yfoapd69?svg=true)](https://ci.appveyor.com/project/fstab/grok-exporter)
* mac OS 64 Bit: Supported [![Build Status](https://travis-ci.org/fstab/grok_exporter.svg?branch=master)](https://travis-ci.org/fstab/grok_exporter)

Grok pattern support:

* We are able to compile all of Grok's default patterns on [github.com/logstash-plugins/logstash-patterns-core](https://github.com/logstash-plugins/logstash-patterns-core/tree/818b7aa60d3c2fea008ea673dbbc49179c6df2c8/patterns).

Prometheus support:

* As of now, we implemented only `counter` as an example of a Prometheus metric. We will implement support for more metric types, as well as metrics to monitor `grok_exporter` itself.

How to Configure Your Own Patterns and Metrics
----------------------------------------------

[CONFIG.md] describes the `grok_exporter` configuration file and shows how to define Grok patterns, Prometheus metrics, and labels.

How to build from source
-----------------------

In order to compile `grok_exporter` from source, you need [Go] installed and `$GOPATH` set, and you need the source files of the [Oniguruma] regular expression library:

On OS X:

```bash
brew install oniguruma
```

On Ubuntu Linux:

```bash
sudo apt-get install libonig-dev
```

Then, download and compile as follows:

```bash
go get github.com/fstab/grok_exporter
cd $GOPATH/src/github.com/fstab/grok_exporter
git submodule update --init --recursive
```

More Documentation
------------------

Implementation notes are available on the [Wiki pages]:

* [tailer (tail -f)](https://github.com/fstab/grok_exporter/wiki/tailer-(tail-%E2%80%90f))
* [About the Regular Expression Library](https://github.com/fstab/grok_exporter/wiki/About-the-Regular-Expression-Library)

Related Projects
----------------

Google's [mtail] goes in a similar direction. It uses its own pattern definition language, so it will not work out-of-the-box with existing Grok patterns. However, `mtail`'s [RE2] regular expressions are probably [more CPU efficient] than Grok's [Oniguruma] patterns. `mtail`'s file tailer seems to have its [focus on Linux].

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
[regexp]: https://golang.org/pkg/regexp
[RE2]: https://github.com/google/re2/wiki/Syntax
[mtail]: https://github.com/google/mtail
[regexp2]: https://github.com/dlclark/regexp2
[pcre]: https://github.com/glenn-brown/golang-pkg-pcre
[libpcre]: http://www.pcre.org
[rubex]: https://github.com/moovweb/rubex
[http://www.apache.org/licenses/LICENSE-2.0]: http://www.apache.org/licenses/LICENSE-2.0
[more CPU efficient]: https://github.com/fstab/grok_exporter/wiki/About-the-Regular-Expression-Library
[focus on Linux]: https://github.com/fstab/grok_exporter/wiki/tailer-(tail-%E2%80%90f)
[Wiki pages]: https://github.com/fstab/grok_exporter/wiki