// Copyright 2016-2019 The grok_exporter Authors
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
	"github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
)

type WebhookTailer struct {
	lines  chan *fswatcher.Line
	errors chan fswatcher.Error
	config *v2.InputConfig
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

func InitWebhookTailer(inputConfig *v2.InputConfig) fswatcher.FileTailer {
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

	lines := WebhookProcessBody(wts.config, b)
	for _, line := range lines {
		logrus.WithFields(logrus.Fields{
			"line": line,
		}).Debug("Groking line")
		lineChan <- &fswatcher.Line{Line: line}
	}
	return
}

func WebhookProcessBody(c *v2.InputConfig, b []byte) []string {

	strs := []string{}

	switch c.WebhookFormat {
	case "text_single":
		s := strings.TrimSpace(string(b))
		strs = append(strs, s)
	case "text_bulk":
		s := strings.TrimSpace(string(b))
		strs = strings.Split(s, c.WebhookTextBulkSeparator)
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
		strs = append(strs, s)
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
			strs = append(strs, s)
		}
	default:
		// error silently
	}

	// Trim whitespace before and after every log entry
	for i, _ := range strs {
		strs[i] = strings.TrimSpace(strs[i])
	}

	return strs
}
