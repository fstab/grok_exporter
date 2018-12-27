// Copyright 2018 The grok_exporter Authors
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

package fswatcher

// uncomment to test watching multiple files manually
// currently, automated tests only cover watching single files, see ../fileTailer_test.go

//func TestFSWatcher(t *testing.T) {
//	var (
//		w   FSWatcher
//		l   Line
//		err error
//	)
//	w, err = Run([]string{"/tmp/test/*"}, true, true)
//	if err != nil {
//		t.Fatal(err)
//	}
//	for {
//		select {
//		case l = <-w.Lines():
//			fmt.Printf("Read line %q from file %q\n", l.Line, l.File)
//		case err = <-w.Errors():
//			if err != nil {
//				t.Fatal(err)
//			} else {
//				t.Fatalf("errors channel closed unexpectedly")
//			}
//		}
//	}
//}
