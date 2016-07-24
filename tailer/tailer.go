package tailer

type Tailer interface {
	Lines() chan string
	Errors() chan error
	Close()
}
