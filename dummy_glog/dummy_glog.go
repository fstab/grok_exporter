package dummy_glog

import (
	"fmt"
	"os"
)

// github.com/google/mtail/tailer depends on github.com/golang/glog.
// However, it doesn't make sense to use glog (including its command line flags) only for one package.
// We mock it for now, and maybe introduce some common logging later.

func Infof(format string, args ...interface{}) {}

func Warningf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func Info(args ...interface{}) {
}

func Error(args ...interface{}) {
	fmt.Fprint(os.Stderr, args...)
}

func V(level interface{}) glog {
	return glog{}
}

type glog struct{}

func (g glog) Infof(format string, args ...interface{}) {
	Infof(format, args)
}
