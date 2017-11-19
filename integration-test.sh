#!/bin/bash

set -e

#####################################################################################
# Test the grok_exporter executable in $GOPATH/bin
#####################################################################################

# Mock cygpath on Linux and OS X, so we can run the same script on all operating systems.
if [[ $(uname -s | tr '[a-z]' '[A-Z]') != *"CYGWIN"* ]] ; then
    function cygpath() {
        echo $2
    }
fi

config_file=$(mktemp /tmp/grok_exporter-test-config.XXXXXX)
log_file=$(mktemp /tmp/grok_exporter-test-log.XXXXXX)

function cleanup_temp_files {
    echo "cleaning up..."
    rm -f $config_file
    rm -f $log_file
}

# clean up on exit
trap cleanup_temp_files EXIT

cat <<EOF > $config_file
global:
    config_version: 2
    retention_check_interval: 100ms
input:
    type: file
    path: $(cygpath -w $log_file)
    readall: true
grok:
    patterns_dir: $(cygpath -w $GOPATH/src/github.com/fstab/grok_exporter/logstash-patterns-core/patterns)
    additional_patterns:
    - 'SERVICE [a-zA-Z_]+'
metrics:
    - type: counter
      name: grok_test_counter_nolabels
      help: Counter metric without labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
    - type: counter
      name: grok_test_counter_labels
      help: Counter metric with labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      labels:
          user: '{{.user}}'
    - type: counter
      name: grok_test_counter_retention
      help: Counter metric with retention
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} retention_test %{NUMBER:val}'
      retention: 1s
      labels:
          user: '{{.user}}'
    - type: gauge
      name: grok_test_gauge_nolabels
      help: Gauge metric without labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
    - type: gauge
      name: grok_test_gauge_labels
      help: Gauge metric with labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      labels:
          user: '{{.user}}'
    - type: gauge
      name: grok_test_gauge_delete
      help: Gauge metric with labels and delete_match
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      labels:
          user: '{{.user}}'
          service: '{{.service}}'
      value: '{{.val}}'
      delete_match: '%{DATE} %{TIME} %{SERVICE:service} shutdown'
      delete_labels:
          service: '{{.service}}'
    - type: gauge
      name: grok_test_gauge_delete_no_labels
      help: Gauge metric with labels and delete_match
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      labels:
          user: '{{.user}}'
          service: '{{.service}}'
      value: '{{.val}}'
      delete_match: '%{DATE} %{TIME} %{SERVICE:service} shutdown'
    - type: histogram
      name: grok_test_histogram_nolabels
      help: Histogram metric without labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      buckets: [1, 2, 3]
    - type: histogram
      name: grok_test_histogram_labels
      help: Histogram metric with labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      buckets: [1, 2, 3]
      labels:
          user: '{{.user}}'
    - type: summary
      name: grok_test_summary_nolabels
      help: Summary metric without labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      value: '{{.val}}'
    - type: summary
      name: grok_test_summary_labels
      help: Summary metric with labels.
      match: '%{DATE} %{TIME} %{SERVICE:service} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          user: '{{.user}}'
server:
    port: 9144
EOF

touch $log_file

$GOPATH/bin/grok_exporter -config $(cygpath -w $config_file) &
pid=$!
disown
trap "kill $pid && cleanup_temp_files" EXIT
sleep 0.25

echo '30.07.2016 14:37:03 service_a alice 1.5' >> $log_file
echo '30.07.2016 14:37:03 service_b alice 1.5' >> $log_file
echo 'some unrelated line' >> $log_file
echo '30.07.2016 14:37:33 service_a alice 2.5' >> $log_file
echo '30.07.2016 14:37:33 service_b alice 2.5' >> $log_file
echo '30.07.2016 14:43:02 service_a bob 2.5' >> $log_file

function checkMetric() {
    val=$(curl -s http://localhost:9144/metrics | grep -v '#' | grep "$1 ") || ( echo "FAILED: Metric '$1' not found." >&2 && exit -1 )
    echo $val | grep "$1 $2" > /dev/null || ( echo "FAILED: Expected '$1 $2', but got '$val'." >&2 && exit -1 )
}

function assertMetricDoesNotExist() {
    set +e
    curl -s http://localhost:9144/metrics | grep -v '#' | grep "$1 " > /dev/null
    if [ $? -eq 0 ] # if metric was found we should fail (we expect the metric to not exist)
    then
        echo "FAILED: Metric '$1' should not exist." >&2
        exit -1
    fi
    set -e
}

echo "Checking metrics..."

# curl -s http://localhost:9144/metrics
# exit

checkMetric 'grok_test_counter_nolabels' 5
checkMetric 'grok_test_counter_labels{user="alice"}' 4
checkMetric 'grok_test_counter_labels{user="bob"}' 1

checkMetric 'grok_test_gauge_nolabels' 2.5
checkMetric 'grok_test_gauge_labels{user="alice"}' 2.5
checkMetric 'grok_test_gauge_labels{user="bob"}' 2.5

checkMetric 'grok_test_histogram_nolabels_bucket{le="1"}' 0
checkMetric 'grok_test_histogram_nolabels_bucket{le="2"}' 2
checkMetric 'grok_test_histogram_nolabels_bucket{le="3"}' 5
checkMetric 'grok_test_histogram_nolabels_bucket{le="+Inf"}' 5
checkMetric 'grok_test_histogram_nolabels_sum' 10.5
checkMetric 'grok_test_histogram_nolabels_count' 5

checkMetric 'grok_test_histogram_labels_bucket{user="alice",le="1"}' 0
checkMetric 'grok_test_histogram_labels_bucket{user="alice",le="2"}' 2
checkMetric 'grok_test_histogram_labels_bucket{user="alice",le="3"}' 4
checkMetric 'grok_test_histogram_labels_bucket{user="alice",le="+Inf"}' 4
checkMetric 'grok_test_histogram_labels_sum{user="alice"}' 8
checkMetric 'grok_test_histogram_labels_count{user="alice"}' 4

checkMetric 'grok_test_histogram_labels_bucket{user="bob",le="1"}' 0
checkMetric 'grok_test_histogram_labels_bucket{user="bob",le="2"}' 0
checkMetric 'grok_test_histogram_labels_bucket{user="bob",le="3"}' 1
checkMetric 'grok_test_histogram_labels_bucket{user="bob",le="+Inf"}' 1
checkMetric 'grok_test_histogram_labels_sum{user="bob"}' 2.5
checkMetric 'grok_test_histogram_labels_count{user="bob"}' 1

checkMetric 'grok_test_summary_nolabels{quantile="0.9"}' 2.5
checkMetric 'grok_test_summary_nolabels_sum' 10.5
checkMetric 'grok_test_summary_nolabels_count' 5

checkMetric 'grok_test_summary_labels{user="alice",quantile="0.9"}' 2.5
checkMetric 'grok_test_summary_labels_sum{user="alice"}' 8
checkMetric 'grok_test_summary_labels_count{user="alice"}' 4

checkMetric 'grok_test_summary_labels{user="bob",quantile="0.9"}' 2.5
checkMetric 'grok_test_summary_labels_sum{user="bob"}' 2.5
checkMetric 'grok_test_summary_labels_count{user="bob"}' 1

# Check built-in metrics (except for processing time and buffer load):

checkMetric 'grok_exporter_lines_total{status="ignored"}' 1
checkMetric 'grok_exporter_lines_total{status="matched"}' 5

checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_counter_labels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_counter_nolabels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_counter_retention"}' 0
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_gauge_labels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_gauge_nolabels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_histogram_labels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_histogram_nolabels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_summary_labels"}' 5
checkMetric 'grok_exporter_lines_matching_total{metric="grok_test_summary_nolabels"}' 5

checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_counter_labels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_counter_nolabels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_counter_retention"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_gauge_labels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_gauge_nolabels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_histogram_labels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_histogram_nolabels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_summary_labels"}' 0
checkMetric 'grok_exporter_line_processing_errors_total{metric="grok_test_summary_nolabels"}' 0

# -----------------------
# Test logrotate
# -----------------------
# simulate logrotate by deleting and re-creating $log_file
rm $log_file
echo '30.07.2016 14:45:59 service_a alice 2.5' >> $log_file

sleep 0.1
echo "Checking metrics..."

checkMetric 'grok_test_counter_nolabels' 6

# -----------------------
# Test deletion of labels
# -----------------------

# Before the shutdown message, the gauge metrics for service_b should be available.
checkMetric 'grok_test_gauge_delete{service="service_b",user="alice"}' 2.5
checkMetric 'grok_test_gauge_delete_no_labels{service="service_b",user="alice"}' 2.5

# The shutdown message should trigger the deletion of all grok_test_gauge_delete metrics with service='service_b'
echo '30.07.2016 14:45:59 service_b shutdown' >> $log_file
# Test if the metrics are gone:
sleep 0.1
# For grok_test_gauge_delete we expect only service_b to be deleted, but service_a should still be there
checkMetric 'grok_test_gauge_delete{service="service_a",user="alice"}' 2.5
set +e
curl -s http://localhost:9144/metrics | grep 'grok_test_gauge_delete' | grep 'service_b'
if [ $? -eq 0 ]
then
    echo "grok_test_gauge_delete is still there, but should have been deleted." >&2
    exit -1
fi
# For grok_test_gauge_delete_no_labels we expect all entries to be gone
curl -s http://localhost:9144/metrics | grep 'grok_test_gauge_delete_no_labels{'
if [ $? -eq 0 ]
then
    echo "grok_test_gauge_delete_no_labels is still there, but should have been deleted." >&2
    exit -1
fi
set -e

# -----------------------
# Test metric retention
# -----------------------

echo '30.07.2016 14:37:33 service_a alice retention_test 2.5' >> $log_file
echo '30.07.2016 14:37:33 service_a bob retention_test 2.5' >> $log_file

checkMetric 'grok_test_counter_retention{user="alice"}' 1
checkMetric 'grok_test_counter_retention{user="bob"}' 1

sleep .7

# Update 'bob' so this metric will not be deleted
echo '30.07.2016 14:37:33 service_a bob retention_test 2.5' >> $log_file

sleep .7

# 'alice' should have been deleted after 1 second, 'bob' is still there because it was updated within the last second
assertMetricDoesNotExist 'grok_test_counter_retention{user="alice"}'
checkMetric 'grok_test_counter_retention{user="bob"}' 2

# Update 'alice', now the metric should re-appear
echo '30.07.2016 14:37:33 service_a alice retention_test 2.5' >> $log_file
sleep 0.1
checkMetric 'grok_test_counter_retention{user="alice"}' 1
checkMetric 'grok_test_counter_retention{user="bob"}' 2

echo SUCCESS
