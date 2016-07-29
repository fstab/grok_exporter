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
input:
    type: file
    path: $(cygpath -w $log_file)
    readall: true
grok:
    patterns_dir: $(cygpath -w $GOPATH/src/github.com/fstab/grok_exporter/logstash-patterns-core/patterns)
    additional_patterns:
    - 'EXIM_MESSAGE [a-zA-Z ]*'
metrics:
    - type: counter
      name: grok_test_example_lines_total
      help: Counter metric without labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
    - type: counter
      name: grok_test_example_lines_total_by_user
      help: Counter metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      labels:
          - grok_field_name: user
            prometheus_label: user
    - type: gauge
      name: grok_test_example_values_total
      help: Gauge metric without labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
    - type: gauge
      name: grok_test_example_values_total_by_user
      help: Gauge metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      labels:
          - grok_field_name: user
            prometheus_label: user
    - type: histogram
      name: grok_test_example_histogram_total
      help: Histogram metric without labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      buckets: [1, 2, 3]
    - type: histogram
      name: grok_test_example_histogram_total_by_user
      help: Histogram metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      buckets: [1, 2, 3]
      labels:
          - grok_field_name: user
            prometheus_label: user
    - type: summary
      name: grok_test_example_summary_total
      help: Summary metric without labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      value: val
    - type: summary
      name: grok_test_example_summary_total_by_user
      help: Summary metric with labels.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          - grok_field_name: user
            prometheus_label: user
server:
    port: 9144
EOF

touch $log_file

$GOPATH/bin/grok_exporter -config $(cygpath -w $config_file) &
pid=$!
disown
trap "kill $pid && cleanup_temp_files" EXIT
sleep 0.25

echo '30.07.2016 14:37:03 alice 1.5' >> $log_file
echo 'some unrelated line' >> $log_file
echo '30.07.2016 14:37:33 alice 2.5' >> $log_file
echo '30.07.2016 14:43:02 bob 2.5' >> $log_file

function checkMetric() {
    val=$(curl -s http://localhost:9144/metrics | grep -v '#' | grep "$1 ") || ( echo "FAILED: Metric '$1' not found." >&2 && exit -1 )
    echo $val | grep "$1 $2" > /dev/null || ( echo "FAILED: Expected '$1 $2', but got '$val'." >&2 && exit -1 )
}

echo "Checking metrics..."

checkMetric 'grok_test_example_lines_total' 3
checkMetric 'grok_test_example_lines_total_by_user{user="alice"}' 2
checkMetric 'grok_test_example_lines_total_by_user{user="bob"}' 1

checkMetric 'grok_test_example_values_total' 6.5
checkMetric 'grok_test_example_values_total_by_user{user="alice"}' 4
checkMetric 'grok_test_example_values_total_by_user{user="bob"}' 2.5

checkMetric 'grok_test_example_histogram_total_bucket{le="1"}' 0
checkMetric 'grok_test_example_histogram_total_bucket{le="2"}' 1
checkMetric 'grok_test_example_histogram_total_bucket{le="3"}' 3

checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="alice",le="1"}' 0
checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="alice",le="2"}' 1
checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="alice",le="3"}' 2
checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="bob",le="1"}' 0
checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="bob",le="2"}' 0
checkMetric 'grok_test_example_histogram_total_by_user_bucket{user="bob",le="3"}' 1

checkMetric 'grok_test_example_summary_total{quantile="0.9"}' 2.5
checkMetric 'grok_test_example_summary_total_sum' 6.5
checkMetric 'grok_test_example_summary_total_count' 3

checkMetric 'grok_test_example_summary_total_by_user{user="alice",quantile="0.9"}' 2.5
checkMetric 'grok_test_example_summary_total_by_user_sum{user="alice"}' 4
checkMetric 'grok_test_example_summary_total_by_user_count{user="alice"}' 2
checkMetric 'grok_test_example_summary_total_by_user{user="bob",quantile="0.9"}' 2.5
checkMetric 'grok_test_example_summary_total_by_user_sum{user="bob"}' 2.5
checkMetric 'grok_test_example_summary_total_by_user_count{user="bob"}' 1

echo "Simulating logrotate..."

rm $log_file
echo '30.07.2016 14:45:59 alice 2.5' >> $log_file

sleep 0.1
echo "Checking metrics..."

checkMetric 'grok_test_example_lines_total' 4

echo SUCCESS
