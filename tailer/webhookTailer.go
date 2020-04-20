// Copyright 2016-2020 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tailer

import (
	"errors"
	json "github.com/bitly/go-simplejson"
	configuration "github.com/fstab/grok_exporter/config/v3"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
)

type context_string struct {
	// The log line itself
	line string
	// Optional extra context to be made available to go templating
	extra map[string]interface{}
}

type WebhookTailer struct {
	lines  chan *fswatcher.Line
	errors chan fswatcher.Error
	config *configuration.InputConfig
}

var webhookTailerSingleton *WebhookTailer

func (t *WebhookTailer) Lines() chan *fswatcher.Line {
	return t.lines
}

func (t *WebhookTailer) Errors() chan fswatcher.Error {
	return t.errors
}

func (t *WebhookTailer) Close() {
	// NO-OP, since the webserver thread is handled by the metrics server
}

func InitWebhookTailer(inputConfig *configuration.InputConfig) fswatcher.FileTailer {
	if webhookTailerSingleton != nil {
		return webhookTailerSingleton
	}

	lineChan := make(chan *fswatcher.Line)
	errorChan := make(chan fswatcher.Error)
	webhookTailerSingleton = &WebhookTailer{
		lines:  lineChan,
		errors: errorChan,
		config: inputConfig,
	}
	return webhookTailerSingleton
}

func WebhookHandler() http.Handler {
	return webhookTailerSingleton
}

func (t WebhookTailer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Implement the http handler interface

	wts := webhookTailerSingleton
	lineChan := wts.lines
	errorChan := wts.errors

	if r.Body == nil {
		err := errors.New("got empty request body")
		logrus.Warn(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "")
		return
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.Warn(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "")
		return
	}
	defer r.Body.Close()

	context_strings := WebhookProcessBody(wts.config, b)
	for _, context_string := range context_strings {
		logrus.WithFields(logrus.Fields{
			"line": context_string.line,
			"extra": context_string.extra,
		}).Debug("Groking line")
		lineChan <- &fswatcher.Line{Line: context_string.line, Extra: context_string.extra}
	}
	return
}

func WebhookProcessBody(c *configuration.InputConfig, b []byte) []context_string {

	strs := []context_string{}

	switch c.WebhookFormat {
	case "text_single":
		s := context_string{line: strings.TrimSpace(string(b))}
		strs = append(strs, s)
	case "text_bulk":
		s := strings.TrimSpace(string(b))
		lines := strings.Split(s, c.WebhookTextBulkSeparator)
		for _, s := range lines {
			strs = append(strs, context_string{line: s})
		}
	case "json_single":
		path := strings.Split(c.WebhookJsonSelector[1:], ".")
		j, err := json.NewJson(b)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"post_body": string(b),
			}).Warn("Unable to Parse JSON")
			break
		}
		s, err := j.GetPath(path...).String()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"post_body":             string(b),
				"webhook_json_selector": c.WebhookJsonSelector,
			}).Warn("Unable to find selector path")
			break
		}
		strs = append(strs, context_string{line: s, extra: j.MustMap()})
	case "json_bulk":
		path := strings.Split(c.WebhookJsonSelector[1:], ".")
		j, err := json.NewJson(b)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"post_body": string(b),
			}).Warn("Unable to Parse JSON")
			break
		}

		for _, ei := range j.MustArray() {
			// Cast the entry interface{} back to the Json object.
			//   Unfortunately, this is how the simplejson lib works.
			ej := json.New()
			ej.Set("x", ei)
			new_path := []string{}
			new_path = append([]string{"x"}, path...)

			s, err := ej.GetPath(new_path...).String()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"post_body":             string(b),
					"webhook_json_selector": c.WebhookJsonSelector,
				}).Warn("Unable to find selector path")
				break
			}
			strs = append(strs, context_string{line: s, extra: ej.MustMap()})
		}
	default:
		// error silently
	}

	// Trim whitespace before and after every log entry
	for i := range strs {
		strs[i] = context_string{line: strings.TrimSpace(strs[i].line), extra: strs[i].extra}
	}

	return strs
}
