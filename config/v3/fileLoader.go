// Copyright 2020 The grok_exporter Authors
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

package v3

import (
	"fmt"
	"github.com/fstab/grok_exporter/tailer/glob"
	"io/ioutil"
	"path/filepath"
)

type ConfigFile struct {
	Path     string
	Contents string
}

type FileLoader interface {
	LoadDir(dir string) ([]*ConfigFile, error)
	LoadGlob(globString string) ([]*ConfigFile, error)
}

type fileLoader struct{}

func NewFileLoader() FileLoader {
	return &fileLoader{}
}

func (f *fileLoader) LoadDir(dir string) ([]*ConfigFile, error) {
	return f.LoadGlob(filepath.Join(dir, "*"))
}

func (f *fileLoader) LoadGlob(globString string) ([]*ConfigFile, error) {
	result := make([]*ConfigFile, 0, 0)
	g, err := glob.Parse(globString)
	if err != nil {
		return nil, err
	}
	fileInfos, err := ioutil.ReadDir(g.Dir())
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(g.Dir(), fileInfo.Name())
		if g.Match(filePath) {
			contents, err := ioutil.ReadFile(filePath)
			if err != nil {
				return nil, err
			}
			result = append(result, &ConfigFile{
				Path:     filePath,
				Contents: string(contents),
			})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%v: file(s) not found", globString)
	}
	return result, nil
}
