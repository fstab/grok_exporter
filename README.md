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

`grok_exporter` is currently just a proof of concept. We are able to compile all of Grok's default patterns, but as of now we implemented only `CounterVec` as an example of a Prometheus metric.

How to run the example
----------------------

An example log file and configuration can be found in the `example` directory. The file `exim-rejected-RCPT-examples.log` contains sample log messages from the [Exim] mail server.
The configuration in `config.yml` extracts the total number of rejected recipients, partitioned by error message.

There is no binary release yet. In order to compile `grok_exporter` from source, you need [Go] installed and `$GOPATH` set, and you need the [Oniguruma] library:

On OS X:

```bash
brew install oniguruma
```

On Ubuntu Linux:

```bash
sudo apt-get install libonig2 libonig-dev
```

Then, download, compile, and run the example as follows:

```bash
go get github.com/fstab/grok_exporter
cd $GOPATH/src/github.com/fstab/grok_exporter
$GOPATH/bin/grok_exporter -config ./example/config.yml
```

The exporter provides the metrics on [https://localhost:8443/metrics]:

![screenshot.png]

About the Regular Expression Library
------------------------------------

[Grok] heavily uses regular expressions in its pattern definitions. Go's built-in [regexp] package implements Google's [RE2] syntax, which is a stripped-down regular expression language.

While RE2 provides some performance guarantees, like a single scan over the input and O(n) execution time with respect to the length of the input, it does only support features that can be modelled as finite state machines (FSM).

In particular, RE2 does not support backtracking and lookahead asseartions, as these cannot be implemented within RE2's performance restrictions.

Grok uses these features a lot, so implementing Grok on top of Go's default [regexp] package is not possible. However, there are a few 3rd party regular expression libraries for Go that do not have these limitations:

* [regexp2] is a port of dotNET's regular expression engine. It is written in pure Go.
* [pcre] is a Go wrapper around the Perl Compatible Regular Expression (PCRE) library (libpcre) (needs `brew install pcre` or `sudo apt-get install libpcre++-dev`)
* [rubex] is a Go wrapper around the [Oniguruma] regular expression library (needs `brew install oniguruma` or `sudo apt-get install libonig2`).

As Grok is originally written in Ruby, and Ruby uses Oniguruma as its regular expression library, we decided to use rubex for best compatibility.

[Prometheus]: https://prometheus.io/
[Grok]: https://www.elastic.co/guide/en/logstash/current/plugins-filters-grok.html
[Logstash]: https://www.elastic.co/products/logstash
[ElasticSearch]: https://www.elastic.co/
[ELK stack]: https://www.elastic.co/webinars/introduction-elk-stack
[Exim]: http://www.exim.org/
[Go]: https://golang.org/
[Oniguruma]: https://github.com/kkos/oniguruma
[screenshot.png]: screenshot.png
[https://localhost:8443/metrics]: https://localhost:8443/metrics
[regexp]: https://golang.org/pkg/regexp
[RE2]: https://github.com/google/re2/wiki/Syntax
[regexp2]: https://github.com/dlclark/regexp2
[pcre]: https://github.com/glenn-brown/golang-pkg-pcre
[libpcre]: http://www.pcre.org
[rubex]: https://github.com/moovweb/rubex
