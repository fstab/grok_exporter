package fswatcher

import "time"

type pollloop struct {
	events chan fsevent
	errors chan Error // unused
	done   chan struct{}
}

func (l *pollloop) Events() chan fsevent {
	return l.events
}

func (l *pollloop) Errors() chan Error {
	return l.errors
}

func (l *pollloop) Close() {
	close(l.done)
}

func runPollLoop(pollInterval time.Duration) *pollloop {

	events := make(chan fsevent)
	errors := make(chan Error) // unused
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()
		for {
			tick := time.After(pollInterval)
			select {
			case <-tick:
				select {
				case events <- struct{}{}:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()
	return &pollloop{
		events: events,
		errors: errors,
		done:   done,
	}
}
