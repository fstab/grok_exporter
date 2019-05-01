package tailer

import (
	"fmt"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// First produce 10,000 lines, then consume 10,000 lines.
func TestLineBufferSequential(t *testing.T) {
	buf := NewLineBuffer()
	defer buf.Close()
	start := time.Now()
	for i := 1; i <= 10000; i++ {
		buf.Push(&fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)})
	}
	fmt.Printf("Producer took %v.\n", time.Since(start))
	start = time.Now()
	for i := 1; i <= 10000; i++ {
		line := buf.BlockingPop()
		if line.Line != fmt.Sprintf("This is line number %v.", i) {
			t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
		}
	}
	fmt.Printf("Consumer took %v.\n", time.Since(start))
}

// Produce and consume in parallel.
func TestLineBufferParallel(t *testing.T) {
	var (
		wg  sync.WaitGroup
		buf = NewLineBuffer()
	)
	defer buf.Close()
	// producer
	go func() {
		start := time.Now()
		for i := 1; i <= 10000; i++ {
			buf.Push(&fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)})
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Producer took %v.\n", time.Since(start))
		wg.Done()
	}()
	// consumer
	go func() {
		start := time.Now()
		for i := 1; i <= 10000; i++ {
			line := buf.BlockingPop()
			if line.Line != fmt.Sprintf("This is line number %v.", i) {
				t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
			}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Consumer took %v.\n", time.Since(start))
		wg.Done()
	}()
	wg.Add(2)
	wg.Wait()
}

func TestLineBufferClear(t *testing.T) {
	buf := NewLineBuffer()
	defer buf.Close()
	for linesInBuffer := 0; linesInBuffer < 10; linesInBuffer++ {
		for i := 1; i <= linesInBuffer; i++ {
			buf.Push(&fswatcher.Line{Line: fmt.Sprintf("This is line number %v of %v.", i, linesInBuffer)})
		}
		if buf.Len() != linesInBuffer {
			t.Fatalf("Expected %v lines in buffer, but got %v", linesInBuffer, buf.Len())
		}
		buf.Clear()
		if buf.Len() != 0 {
			t.Fatalf("Expected %v lines in buffer, but got %v", 0, buf.Len())
		}
	}
}

func TestLineBufferBlockingPop(t *testing.T) {
	buf := NewLineBuffer()
	done := make(chan struct{})
	go func() {
		l := buf.BlockingPop()
		if l.Line != "hello" {
			t.Fatalf("expected to read \"hello\" but got %q.", l.Line)
		}
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("BlockingPop() returned unexpectedly")
	case <-time.After(200 * time.Millisecond):
		// ok
	}
	buf.Push(&fswatcher.Line{Line: "hello", File: "/tmp/hello.log"})
	select {
	case <-done:
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("BlockingPop() not interrupted by Close()")
	}
}

func TestLineBufferClose(t *testing.T) {
	buf := NewLineBuffer()
	done := make(chan struct{})
	go func() {
		buf.BlockingPop()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("BlockingPop() returned unexpectedly")
	case <-time.After(200 * time.Millisecond):
		// ok
	}
	buf.Close() // should interrupt BlockingPop()
	select {
	case <-done:
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("BlockingPop() not interrupted by Close()")
	}
}
