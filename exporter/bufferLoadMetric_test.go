package exporter

import (
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func TestBufferLoadMetric(t *testing.T) {
	m := NewBufferLoadMetric(logrus.New(), false)
	c := make(chan time.Time)
	tick := &time.Ticker{
		C: c,
	}
	f := make(chan struct{})
	m.start(tick, f)
	defer m.Stop()
	defer close(f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  0  |  0  |  0  |  0
	// max |  0  |  0  |  0  |  0

	m.Inc() // cur = 1
	m.Inc() // cur = 2

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  0  |  0  |  0  |  0
	// max |  2  |  2  |  2  |  2

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  2  |  0  |  0  |  0
	// max |  2  |  2  |  2  |  2

	m.Dec() // cur = 1
	m.Inc() // cur = 2

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  1  |  0  |  0  |  0
	// max |  2  |  2  |  2  |  2

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  2  |  1  |  0  |  0
	// max |  2  |  2  |  2  |  2

	m.Inc() // cur = 3
	m.Dec() // cur = 2

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  2  |  1  |  0  |  0
	// max |  3  |  3  |  3  |  3

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  2  |  2  |  1  |  0
	// max |  2  |  3  |  3  |  3

	expectValues(t, m, 2, 2, 1, 0, 2, 3, 3, 3)

	m.Inc() // cur = 3
	m.Inc() // cur = 4
	m.Inc() // cur = 5
	m.Dec() // cur = 4

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  2  |  2  |  1  |  0
	// max |  5  |  5  |  5  |  5

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  4  |  2  |  2  |  1
	// max |  4  |  5  |  5  |  5

	m.Inc() // cur = 5

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  4  |  2  |  2  |  1
	// max |  5  |  5  |  5  |  5

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  5  |  4  |  2  |  2
	// max |  5  |  5  |  5  |  5

	m.Dec() // cur = 4

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  4  |  4  |  2  |  2
	// max |  5  |  5  |  5  |  5

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  4  |  4  |  4  |  2
	// max |  4  |  5  |  5  |  5

	expectValues(t, m, 4, 4, 4, 2, 4, 5, 5, 5)
}

func expectValues(t *testing.T, m *bufferLoadMetric, min15s, min30s, min45s, min60s, max15s, max30s, max45s, max60s int64) {
	if m.min15s != min15s {
		t.Fatalf("expected min15s=%v but got min15s=%v", min15s, m.min15s)
	}
	if m.min30s != min30s {
		t.Fatalf("expected min30s=%v but got min30s=%v", min30s, m.min30s)
	}
	if m.min45s != min45s {
		t.Fatalf("expected min45s=%v but got min45s=%v", min45s, m.min45s)
	}
	if m.min60s != min60s {
		t.Fatalf("expected min60s=%v but got min60s=%v", min60s, m.min60s)
	}
	if m.max15s != max15s {
		t.Fatalf("expected max15s=%v but got max15s=%v", max15s, m.max15s)
	}
	if m.max30s != max30s {
		t.Fatalf("expected max30s=%v but got max30s=%v", max30s, m.max30s)
	}
	if m.max45s != max45s {
		t.Fatalf("expected max45s=%v but got max45s=%v", max45s, m.max45s)
	}
	if m.max60s != max60s {
		t.Fatalf("expected max60s=%v but got max60s=%v", max60s, m.max60s)
	}
}

func TestResetBufferLoadMetrics(t *testing.T) {
	m := NewBufferLoadMetric(logrus.New(), false)
	c := make(chan time.Time)
	tick := &time.Ticker{
		C: c,
	}
	f := make(chan struct{})
	m.start(tick, f)
	defer m.Stop()
	defer close(f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  0  |  0  |  0  |  0
	// max |  0  |  0  |  0  |  0

	m.Inc() // cur = 1
	m.Inc() // cur = 2
	m.Inc() // cur = 3
	m.Inc() // cur = 4
	m.Inc() // cur = 5

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  0  |  0  |  0  |  0
	// max |  5  |  5  |  5  |  5

	synchronousTick(c, f)
	synchronousTick(c, f)
	synchronousTick(c, f)
	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  5  |  5  |  5  |  5
	// max |  5  |  5  |  5  |  5

	m.Set(7)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  5  |  5  |  5  |  5
	// max |  7  |  7  |  7  |  7

	expectValues(t, m, 5, 5, 5, 5, 7, 7, 7, 7)

	m.Set(1)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  1  |  1  |  1  |  1
	// max |  7  |  7  |  7  |  7

	expectValues(t, m, 1, 1, 1, 1, 7, 7, 7, 7)

	synchronousTick(c, f)

	//     | 15s | 30s | 45s | 60s
	// ----------------------------
	// min |  1  |  1  |  1  |  1
	// max |  1  |  7  |  7  |  7

	expectValues(t, m, 1, 1, 1, 1, 1, 7, 7, 7)
}

func synchronousTick(c chan time.Time, f chan struct{}) {
	c <- time.Now()
	<-f // wait until tick is processed
}
