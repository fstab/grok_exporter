package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

func main() {

	logdir := fmt.Sprintf("%s%cgrok_exporter-load-test", os.Getenv("HOME"), os.PathSeparator)
	mkdir(logdir)

	done := make(chan struct{})

	for i, config := range []struct {
		logfile        string
		linesPerMinute int
	}{
		{
			logfile:        "access.log",
			linesPerMinute: 120,
		},
		{
			logfile:        "error.log",
			linesPerMinute: 120,
		},
		{
			logfile:        "server1.log",
			linesPerMinute: 60,
		},
		{
			logfile:        "server2.log",
			linesPerMinute: 60,
		},
		{
			logfile:        "server3.log",
			linesPerMinute: 60,
		},
	} {
		logger := logrus.New()
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		})
		logger.SetReportCaller(true)
		fields := logrus.Fields{
			"hostname": "staging-1",
			"appname":  "foo-app",
			"session":  "1ce3f6v",
			"string":   "hello",
			"int":      i,
			"float":    float64(i) + 0.1,
		}
		filename := fmt.Sprintf("%s%c%s", logdir, os.PathSeparator, config.logfile)
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", filename, err)
			os.Exit(1)
		}
		logger.SetOutput(file)

		go func() {
			for _ = range time.Tick(time.Minute / time.Duration(config.linesPerMinute)) {
				logger.WithFields(fields).WithFields(logrus.Fields{"string": "foo", "int": 1, "float": 1.1}).Info("My first ssl event from Golang")
			}
		}()
	}
	<-done // wait forever
}

func mkdir(dir string) {
	var err error
	if _, err = os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(dir, 0755)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: failed to create directory: %v\n", dir, err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			os.Exit(1)
		}
	}
}
