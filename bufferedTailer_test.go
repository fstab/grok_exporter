package main

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type sourceTailer struct {
	lines chan string
}

func (tail *sourceTailer) LineChan() chan string {
	return tail.lines
}

func (tail *sourceTailer) ErrorChan() chan error {
	return nil
}

func (tail *sourceTailer) Close() {
	close(tail.lines)
}

func TestSequential(t *testing.T) {
	src := &sourceTailer{lines: make(chan string)}
	buffered := BufferedTailerWithMetrics(src)
	for i := 0; i < 10000; i++ {
		src.lines <- fmt.Sprintf("This is line number %v.", i)
	}
	for i := 0; i < 10000; i++ {
		line := <-buffered.LineChan()
		if line != fmt.Sprintf("This is line number %v.", i) {
			t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
		}
	}
	buffered.Close()
	_, stillOpen := <-buffered.LineChan()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.LineChan()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
}

func TestParallel(t *testing.T) {
	src := &sourceTailer{lines: make(chan string)}
	buffered := BufferedTailerWithMetrics(src)
	var wg sync.WaitGroup
	go func() {
		start := time.Now()
		for i := 0; i < 10000; i++ {
			src.lines <- fmt.Sprintf("This is line number %v.", i)
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Producer took %v.\n", time.Since(start))
		wg.Done()
	}()
	go func() {
		start := time.Now()
		for i := 0; i < 10000; i++ {
			line := <-buffered.LineChan()
			if line != fmt.Sprintf("This is line number %v.", i) {
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
	buffered.Close()
	_, stillOpen := <-buffered.LineChan()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.LineChan()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
}
