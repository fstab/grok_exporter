Built-In Metrics
================

In addition to the metrics defined in the [configuration file], `grok_exporter` provides some metrics out of the box:

grok_exporter_lines_total
-------------------------

Counts the number of log lines processed by grok_exporter, partitioned by the line's status:

* `ignored`: The line did not match any metrics from the configuration file.
* `matched`: The line matched at least one metric from the configuration file.

grok_exporter_lines_matching_total
----------------------------------

Counts the number of matching log lines, partitioned by the metrics from the configuration file. Note that one log line can match multiple metrics, so `sum(grok_exporter_lines_matching_total) by (instance, job)` might be greater than `grok_exporter_lines_total{status="matched"}`.

grok_exporter_lines_processing_time_microseconds_total
------------------------------------------------------

Counts the processing time for log lines in microseconds, partitioned by the metrics from the configuration file. This metric sums up the processing times for all matched lines. To get the average processing time for a single line, divide `grok_exporter_lines_processing_time_microseconds_total / grok_exporter_lines_matching_total`.

grok_exporter_line_processing_errors_total
------------------------------------------

Counts the number of line processing errors, partitioned by the metrics from the configuration file. Errors can only occur if there is a misconfiguration. For example, an error occurs if a Gauge/Histogram/Summary metric has a value that does not match a valid number. In that case, you should modify the Grok expression to make sure that the value always matches a valid number. If an error occurs, the line causing the error is printed to the console, together with information what went wrong.

grok_exporter_line_buffer_peak_load
-----------------------------------

When lines are read from a log file, they are stored in an in-memory buffer before they are evaluated. That way, `grok_exporter` can temporarily read new lines faster than it processes lines. If lines are continuously read faster than they are processed, the buffer will eventually run out of memory.

Each second, `grok_exporter` captures the peak load of the line buffer during the last second (i.e. the maximum number of lines in the line buffer during the last second). These peak loads are exposed through the `grok_exporter_line_buffer_peak_load` metric.

This metric is work in progress. The goal is to configure an alert when `grok_exporter` processes lines too slowly and may run out of memory. However, we still need to figure out if `grok_exporter_line_buffer_peak_load` is a good indicator for that.

grok_exporter_build_info
------------------------

A metric with constant value `1` providing the following labels:

* `version`: Version of the grok_exporter.
* `builddate`: Date when the grok_exporter was built, format _YYYY-MM-DD_.
* `branch`: Git branch from which the grok_exporter was built, e.g. _master_ .
* `revision`: Git commit from which the grok_exporter was built.
* `goversion`: Version of the Go compiler used for building the grok_eporter.
* `platform`: Operating system and architecture for which the grok_exporter was built, e.g. _linux-amd64_.

See [exposing the software version to Prometheus on robustperception.io] to learn more about this approach.

[configuration file]: CONFIG.md
[exposing the software version to Prometheus on robustperception.io]: http://www.robustperception.io/exposing-the-software-version-to-prometheus/
