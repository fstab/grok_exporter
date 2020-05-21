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

package exporter

import (
	"bufio"
	"fmt"
	"github.com/fstab/grok_exporter/tailer/glob"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Patterns map[string]string

func InitPatterns() *Patterns {
	result := Patterns(make(map[string]string))
	return &result
}

func (p *Patterns) AddDir(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read pattern directory %v: %v", path, err)
	}
	for _, file := range files {
		err = p.AddFile(filepath.Join(path, file.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Patterns) AddGlob(globString string) error {
	nFilesFound := 0
	nDirsFound := 0
	g, err := glob.Parse(globString)
	if err != nil {
		return err
	}
	fileInfos, err := ioutil.ReadDir(g.Dir())
	if err != nil {
		return fmt.Errorf("failed to read patterns from directory %v: %v", g.Dir(), err)
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(g.Dir(), fileInfo.Name())
		if g.Match(filePath) {
			if fileInfo.IsDir() {
				// dirs are skipped, but we track this for better error messages
				nDirsFound++
			} else {
				err = p.AddFile(filePath)
				if err != nil {
					return err
				}
				nFilesFound++
			}
		}
	}
	if nFilesFound == 0 {
		if nDirsFound == 1 {
			return fmt.Errorf("%v: expected a file, but found a directory. configuration should be 'dir' instead of 'file'", globString)
		} else {
			return fmt.Errorf("%v: no such file", globString)
		}
	}
	return nil
}

// pattern files see https://github.com/logstash-plugins/logstash-patterns-core/tree/master/patterns
func (p *Patterns) AddFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read pattern file %v: %v", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !isEmpty(line) && !isComment(line) {
			err = p.AddPattern(scanner.Text())
			if err != nil {
				return fmt.Errorf("failed to read pattern file %v: %v", path, err)
			}
		}
	}
	if scanner.Err() != nil {
		return fmt.Errorf("failed to read pattern file %v: %v", path, scanner.Err())
	}
	return nil
}

func (p *Patterns) AddPattern(pattern string) error {
	r := regexp.MustCompile(`([A-z0-9]+)\s+(.+)`)
	match := r.FindStringSubmatch(pattern)
	if match == nil {
		return fmt.Errorf("'%v' is not a valid pattern definition", pattern)
	}
	(*p)[match[1]] = match[2]
	return nil
}

func (p *Patterns) Find(pattern string) (string, bool) {
	result, exists := (*p)[pattern]
	return result, exists
}

func isEmpty(line string) bool {
	return len(line) == 0
}

func isComment(line string) bool {
	return len(line) > 0 && line[0] == '#'
}
