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

How to run the example
----------------------

An example log file and configuration can be found in the `example` directory. The file `exim-rejected-RCPT-examples.log` contains sample log messages from the [Exim] mail server.
The configuration in `config.yml` counts the total number of rejected recipients, partitioned by error message.

In order to run the example, download `grok_exporter-$ARCH.zip` for your operating system from the [releases] page, extract the archive, `cd grok_exporter-$ARCH`, then run

```bash
grok_exporter -config ./example/config.yml
```

The exporter provides the metrics on [http://localhost:9144/metrics]:

![screenshot.png]

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

How to Configure Your Own Patterns and Metrics
----------------------------------------------

[CONFIG.md] describes the `grok_exporter` configuration file and shows how to define Grok patterns, Prometheus metrics, and labels.

Related Projects
----------------

Google's [mtail] goes in a similar direction. It uses [RE2] regular expressions, which is a stripped-down regular expression language. It will not be possible to re-use existing Grok definitions with `mtail`. However, `mtail` is probably more CPU efficient than `grok_exporter`. We will provide some benchmarks soon.

About the Regular Expression Library
------------------------------------

[Grok] heavily uses regular expressions in its pattern definitions. Go's built-in [regexp] package implements Google's [RE2] syntax, which is a stripped-down regular expression language.

While RE2 provides some performance guarantees, like a single scan over the input and O(n) execution time with respect to the length of the input, it does only support features that can be modelled as finite state machines (FSM).

In particular, RE2 does not support backtracking and lookahead asseartions, as these cannot be implemented within RE2's performance restrictions.

Grok uses these features a lot, so implementing Grok on top of Go's default [regexp] package is not possible. However, there are a few 3rd party regular expression libraries for Go that do not have these limitations:

* [regexp2] is a port of dotNET's regular expression engine. It is written in pure Go.
* [pcre] is a Go wrapper around the Perl Compatible Regular Expression (PCRE) library (libpcre) (needs `brew install pcre` or `sudo apt-get install libpcre++-dev`)
* [rubex] is a Go wrapper around the [Oniguruma] regular expression library (needs `brew install oniguruma` or `sudo apt-get install libonig-dev`).

As Grok is originally written in Ruby, and Ruby uses Oniguruma as its regular expression library, we decided to use rubex for best compatibility.

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
