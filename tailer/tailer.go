package tailer

type Tailer interface {
	LineChan() chan string
	ErrorChan() chan error
	Close()
}
