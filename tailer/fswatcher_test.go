// Copyright 2019 The grok_exporter Authors
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
	"fmt"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"
)

const tests = `
- name: single logfile
  commands:
  - [mkdir, logdir]
  - [log, test line 1, logdir/logfile.log]
  - [log, test line 2, logdir/logfile.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, logdir/logfile.log]
  - [expect, test line 1, logdir/logfile.log]
  - [expect, test line 2, logdir/logfile.log]
  - [log, test line 3, logdir/logfile.log]
  - [expect, test line 3, logdir/logfile.log]
  - [logrotate, logdir/logfile.log, logdir/logfile.log.1]
  - [log, test line 4, logdir/logfile.log]
  - [expect, test line 4, logdir/logfile.log]
  - [log, test line 5, logdir/logfile.log]
  - [expect, test line 5, logdir/logfile.log]

- name: multiple log files, single directory
  commands:
  - [mkdir, logdir]
  - [log, test line 1 logfile 1, logdir/logfile1.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, logdir/*.log]
  - [expect, test line 1 logfile 1, logdir/logfile1.log]
  - [log, test line 2 logfile 1, logdir/logfile1.log]
  - [log, test line 1 logfile 2, logdir/logfile2.log]
  - [log, test line 2 logfile 2, logdir/logfile2.log]
  - [expect, test line 2 logfile 1, logdir/logfile1.log]
  - [expect, test line 1 logfile 2, logdir/logfile2.log]
  - [expect, test line 2 logfile 2, logdir/logfile2.log]
  - [logrotate, logdir/logfile1.log, logdir/logfile1.log.1]
  - [logrotate, logdir/logfile2.log, logdir/logfile2.log.1]
  - [log, test line 3 logfile 2, logdir/logfile2.log]
  - [log, test line 3 logfile 1, logdir/logfile1.log]
  - [expect, test line 3 logfile 2, logdir/logfile2.log]
  - [expect, test line 3 logfile 1, logdir/logfile1.log]
  - [log, test line 4 logfile 2, logdir/logfile2.log]
  - [expect, test line 4 logfile 2, logdir/logfile2.log]

- name: multiple directories
  commands:
  - [mkdir, logdir1]
  - [mkdir, logdir2]
  - [log, test line 1 dir 1 file 1, logdir1/logfile-1.log]
  - [log, test line 1 dir 2 file 1, logdir2/logfile-1.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, logdir1/*.log, logdir2/*.log]
  - [log, test line 1 dir 1 file 2, logdir1/logfile-2.log]
  - [log, test line 1 dir 2 file 2, logdir2/logfile-2.log]
  - [log, test line 2 dir 1 file 1, logdir1/logfile-1.log]
  - [log, test line 2 dir 1 file 2, logdir1/logfile-2.log]
  - [log, test line 2 dir 2 file 1, logdir2/logfile-1.log]
  - [log, test line 2 dir 2 file 2, logdir2/logfile-2.log]
  - [expect, test line 1 dir 1 file 1, logdir1/logfile-1.log]
  - [expect, test line 1 dir 1 file 2, logdir1/logfile-2.log]
  - [expect, test line 1 dir 2 file 1, logdir2/logfile-1.log]
  - [expect, test line 1 dir 2 file 2, logdir2/logfile-2.log]
  - [expect, test line 2 dir 1 file 1, logdir1/logfile-1.log]
  - [expect, test line 2 dir 1 file 2, logdir1/logfile-2.log]
  - [expect, test line 2 dir 2 file 1, logdir2/logfile-1.log]
  - [expect, test line 2 dir 2 file 2, logdir2/logfile-2.log]
  - [logrotate, logdir1/logfile-1.log, logdir1/logfile-1.log.1]
  - [logrotate, logdir2/logfile-1.log, logdir2/logfile-1.log.1]
  - [log, test line 3 dir 1 file 1, logdir1/logfile-1.log]
  - [log, test line 3 dir 1 file 2, logdir1/logfile-2.log]
  - [log, test line 3 dir 2 file 1, logdir2/logfile-1.log]
  - [log, test line 3 dir 2 file 2, logdir2/logfile-2.log]
  - [expect, test line 3 dir 1 file 1, logdir1/logfile-1.log]
  - [expect, test line 3 dir 1 file 2, logdir1/logfile-2.log]
  - [expect, test line 3 dir 2 file 1, logdir2/logfile-1.log]
  - [expect, test line 3 dir 2 file 2, logdir2/logfile-2.log]

- name: nested directories
  commands:
  - [mkdir, outer]
  - [mkdir, outer/inner]
  - [log, outer line 1, outer/logfile.log]
  - [log, inner line 1, outer/inner/logfile.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, outer/*.log, outer/inner/*.log]
  - [expect, outer line 1, outer/logfile.log]
  - [expect, inner line 1, outer/inner/logfile.log]
  - [log, outer line 2, outer/logfile.log]
  - [log, inner line 2, outer/inner/logfile.log]
  - [expect, outer line 2, outer/logfile.log]
  - [expect, inner line 2, outer/inner/logfile.log]
  - [logrotate, outer/logfile.log, outer/logfile.log.1]
  - [logrotate, outer/inner/logfile.log, outer/inner/logfile.log.1]
  - [log, outer line 3, outer/logfile.log]
  - [log, inner line 3, outer/inner/logfile.log]
  - [expect, outer line 3, outer/logfile.log]
  - [expect, inner line 3, outer/inner/logfile.log]

- name: watch after logrotate
  commands:
  - [mkdir, logdir]
  - [log, line 1, logdir/logfile.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, logdir/*]
  - [expect, line 1, logdir/logfile.log]
  - [log, line 2, logdir/logfile.log]
  - [expect, line 2, logdir/logfile.log]
  - [logrotate, logdir/logfile.log, logdir/logfile.log.1]
  - [log, line 3, logdir/logfile.log]
  - [expect, line 3, logdir/logfile.log]

- name: overwrite
  param_filters:
    logrotateMvCfg: [mv]
  commands:
  - [mkdir, logdir]
  - [log, file 1 line 1, logdir/logfile1.log]
  - [log, file 2 line 1, logdir/logfile2.log]
  - [start file tailer, readall=true, fail_on_missing_logfile=false, logdir/*]
  - [expect, file 1 line 1, logdir/logfile1.log]
  - [expect, file 2 line 1, logdir/logfile2.log]
  - [logrotate, logdir/logfile1.log, logdir/logfile2.log]
     # When processing logratate, the old fd for logfile1.log will be closed and un-watched, and a new fd will be created and watched (on Darwin).
     # Wait a little so that the events for logging line 2 is received on the new fd.
  - [sleep, 200]
  - [log, file 2 line 2, logdir/logfile2.log]
  - [log, file 1 line 2, logdir/logfile1.log]
  - [expect, file 1 line 2, logdir/logfile1.log]
  - [expect, file 2 line 2, logdir/logfile2.log]
`

// // The following test fails on Windows in tearDown() when removing logdir.
// // This is a known bug, we currently ignore this for sys.GOOS = "windows" in tearDown().
// const tests = `
// - name: single logfile
//   commands:
//   - [mkdir, logdir]
//   - [start file tailer, fail_on_missing_logfile=false, logdir/logfile.log]
// `

type testConfigType struct {
	Name         string
	ParamFilters map[string][]string `yaml:"param_filters"`
	Commands     [][]string
}

func TestAll(t *testing.T) {
	testConfigs := make([]testConfigType, 0)
	err := yaml.UnmarshalStrict([]byte(tests), &testConfigs)
	if err != nil {
		t.Fatal(err)
	}
	for _, testConfig := range testConfigs {
		for _, tailerOpt := range []fileTailerConfig{fseventTailer, pollingTailer} {
			loggerCfg := closeFileAfterEachLine
			// All logratate configs except for copy and copytruncate can be combined with logratateMoveConfig.
			for _, logrotateCfg := range []logrotateConfig{_create, _nocreate, _create_from_temp} {
				for _, logrotateMvCfg := range []logrotateMoveConfig{mv, cp, rm} {
					if !skip(testConfig, loggerCfg, logrotateCfg, logrotateMvCfg) {
						runTest(t, testConfig.Name, loggerCfg, tailerOpt, logrotateCfg, logrotateMvCfg, testConfig.Commands)
					}
				}
			}
			for _, loggerCfg := range []loggerConfig{keepOpen, closeFileAfterEachLine} {
				// When the logger keeps the file open, only the logrotate options 'copy' and 'copytruncate' make sense.
				for _, logrotateCfg := range []logrotateConfig{_copy, _copytruncate} {
					for _, logrotateMvCfg := range []logrotateMoveConfig{none} {
						// The logroatate configs copytruncate and copy require logroatateMoveConfig == none.
						if !skip(testConfig, loggerCfg, logrotateCfg, logrotateMvCfg) {
							runTest(t, testConfig.Name, loggerCfg, tailerOpt, logrotateCfg, logrotateMvCfg, testConfig.Commands)
						}
					}
				}
			}
		}
	}
}

// On Mac OS, we receive an additional NOTE_ATTRIB event for each change on the file unless
// the file is located a "hidden" directory. This is probably because Mac OS updates attributes
// used for showing the files in Finder. This does not happen for files in hidden directories:
// * directories starting with a dot are hidden
// * directories with the xattr com.apple.FinderInfo (like everything in /tmp) are hidden
// In order to test this, we must create a log file somewhere outside of /tmp, so we use $HOME.
func TestVisibleInOSXFinder(t *testing.T) {
	ctx := setUp(t, "visible in macOS finder", closeFileAfterEachLine, fseventTailer, _nocreate, mv)

	// replace ctx.basedir with a directory in $HOME
	deleteRecursively(t, ctx, ctx.basedir)
	currentUser, err := user.Current()
	if err != nil {
		fatalf(t, ctx, "failed to get current user: %v", err)
	}
	testDir, err := ioutil.TempDir(currentUser.HomeDir, "grok_exporter_test_dir_")
	if err != nil {
		fatalf(t, ctx, "failed to create test directory: %v", err.Error())
	}
	ctx.basedir = testDir
	defer tearDown(t, ctx)

	// run simple test in the new directory
	test := [][]string{
		{"log", "line 1", "test.log"},
		{"start file tailer", "test.log"},
		{"sleep", "1000"}, // wait a second before we write line 2, because we started the tailer with readall=false
		{"log", "line 2", "test.log"},
		{"expect", "line 2", "test.log"},
		{"sleep", "5000"}, // On macOS, we get a delayed NOTE_ATTRIB event after we wrote 'line 2'. Wait 5 seconds for this event.
		{"log", "line 3", "test.log"},
		{"expect", "line 3", "test.log"},
	}
	executeCommands(t, ctx, test)
}

// test the "fail_on_missing_logfile: false" configuration
func TestFileMissingOnStartup(t *testing.T) {
	test := [][]string{
		{"start file tailer", "fail_on_missing_logfile=false", "test.log"},
		{"sleep", "1000"},
		{"log", "line 1", "test.log"},
		{"expect", "line 1", "test.log"},
	}
	runTest(t, "fail on missing startup", closeFileAfterEachLine, fseventTailer, _nocreate, mv, test)
}

func skip(config testConfigType, loggerCfg loggerConfig, logrotateCfg logrotateConfig, logrotateMvCfg logrotateMoveConfig) bool {
	if len(config.ParamFilters["loggerCfg"]) > 0 && !containsAsString(loggerCfg, config.ParamFilters["loggerCfg"]) {
		return true
	}
	if len(config.ParamFilters["logrotateCfg"]) > 0 && !containsAsString(logrotateCfg, config.ParamFilters["logrotateCfg"]) {
		return true
	}
	if len(config.ParamFilters["logrotateMvCfg"]) > 0 && !containsAsString(logrotateMvCfg, config.ParamFilters["logrotateMvCfg"]) {
		return true
	}
	return false
}

func containsAsString(item fmt.Stringer, slice []string) bool {
	for _, entry := range slice {
		if item.String() == entry {
			return true
		}
	}
	return false
}

func runTest(t *testing.T, testName string, loggerCfg loggerConfig, tailerCfg fileTailerConfig, logrotateCfg logrotateConfig, logrotateMvCfg logrotateMoveConfig, cmds [][]string) {
	params := []fmt.Stringer{loggerCfg, tailerCfg, logrotateCfg, logrotateMvCfg}
	join(params, ",")
	t.Run(testName+"("+join(params, ",")+")", func(t *testing.T) {
		fmt.Println()
		ctx := setUp(t, testName, loggerCfg, tailerCfg, logrotateCfg, logrotateMvCfg)
		defer tearDown(t, ctx)
		executeCommands(t, ctx, cmds)
	})
}

func executeCommands(t *testing.T, ctx *context, cmds [][]string) {
	nGoroutinesBefore := runtime.NumGoroutine()
	for _, cmd := range cmds {
		exec(t, ctx, cmd)
	}
	// The "watch after logrotate" test watches logdir/* and rotates logdir/logfile.log
	// to logdir/logfile.log.1. As a result, the file is still watched after it is rotated.
	// Depending on the logrotate config the lines are read again (cp) or not (mv).
	// We ignore unexpected lines for that test.
	// TODO: Make ignoreUnexpectedLines an explicit paramter in the test yaml instead of using the test name here.
	closeTailer(t, ctx, ctx.testName == "watch after logrotate")
	assertGoroutinesTerminated(t, ctx, nGoroutinesBefore)
	for _, writer := range ctx.logFileWriters {
		writer.close(t, ctx)
	}
}

func closeTailer(t *testing.T, ctx *context, ignoreUnexpectedLines bool) {
	// Note: This function checks if the Lines() channel gets closed.
	// While it's good to check this, it doesn't guarantee that the tailer is
	// fully shut down. There might be an fseventProducerLoop running in the
	// background, or a hanging system call keeping the log directory open.
	// There are tests for that like counting the number of goroutines
	// in assertGoroutinesTerminated() or making sure the log directory
	// can be removed in tearDown().
	timeout := 5 * time.Second
	if ctx.tailer != nil {
		ctx.tailer.Close()
		// check if the lines channel gets closed
		select {
		case line, open := <-ctx.tailer.Lines():
			if open && !ignoreUnexpectedLines {
				fatalf(t, ctx, "read unexpected line line from file %q: %q", line.File, line.Line)
			}
		case <-time.After(timeout):
			fatalf(t, ctx, "failed to shut down the tailer. timeout after %v seconds", timeout)
		}
	}
}

func assertGoroutinesTerminated(t *testing.T, ctx *context, nGoroutinesBefore int) {
	// Timeout of 2 seconds, because after FileTailer.Close() returns the tailer is still
	// shutting down in the background.
	timeout := 2 * time.Second
	for nGoroutinesBefore < runtime.NumGoroutine() && timeout > 0 {
		timeout = timeout - 50*time.Millisecond
		time.Sleep(50 * time.Millisecond)
	}
	nHangingGoroutines := runtime.NumGoroutine() - nGoroutinesBefore
	if nHangingGoroutines > 0 {
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		fatalf(t, ctx, "%v goroutine(s) did not shut down properly.", nHangingGoroutines)
	}
}

func setUp(t *testing.T, testName string, loggerCfg loggerConfig, tailerCfg fileTailerConfig, logrotateCfg logrotateConfig, logrotateMvCfg logrotateMoveConfig) *context {
	ctx := &context{
		logFileWriters: make(map[string]logFileWriter),
		testName:       testName,
		loggerCfg:      loggerCfg,
		tailerCfg:      tailerCfg,
		logrotateCfg:   logrotateCfg,
		logrotateMvCfg: logrotateMvCfg,
	}
	logger := logrus.New()
	logger.Level = logrus.DebugLevel
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		FullTimestamp:   true,
	})
	ctx.log = logger.WithField("test", testName).WithField("params", params(ctx))
	ctx.basedir = mkTempDir(t, ctx)
	return ctx
}

func params(ctx *context) string {
	params := []fmt.Stringer{ctx.loggerCfg, ctx.tailerCfg, ctx.logrotateCfg, ctx.logrotateMvCfg}
	return join(params, ",")
}

type context struct {
	basedir         string
	logFileWriters  map[string]logFileWriter // path -> writer
	testName        string
	loggerCfg       loggerConfig
	tailerCfg       fileTailerConfig
	logrotateCfg    logrotateConfig
	logrotateMvCfg  logrotateMoveConfig
	log             logrus.FieldLogger
	tailer          fswatcher.FileTailer
	linesFromTailer *linesFromTailer
}

func exec(t *testing.T, ctx *context, cmd []string) {
	ctx.log.Debug(printCmd(cmd))
	switch cmd[0] {
	case "mkdir":
		mkdir(t, ctx, cmd[1])
	case "log":
		writer, exists := ctx.logFileWriters[cmd[2]]
		if !exists {
			writer = newLogFileWriter(t, ctx, path.Join(ctx.basedir, cmd[2]))
			ctx.logFileWriters[cmd[2]] = writer
		}
		writer.writeLine(t, ctx, cmd[1])
	case "start file tailer":
		startFileTailer(t, ctx, cmd[1:])
	case "expect":
		expect(t, ctx, cmd[1], cmd[2])
	case "logrotate":
		rotate(t, ctx, cmd[1], cmd[2])
	case "sleep":
		duration, err := strconv.Atoi(cmd[1])
		if err != nil {
			fatalf(t, ctx, "syntax error in test: sleep %v: %v", cmd[1], err)
		}
		time.Sleep(time.Duration(duration) * time.Millisecond)
	default:
		fatalf(t, ctx, "unknown command: %v", printCmd(cmd))
	}
}

func rotate(t *testing.T, ctx *context, from string, to string) {
	fullpath := path.Join(ctx.basedir, from)
	fromDir := filepath.Dir(fullpath)
	filenameFrom := filepath.Base(fullpath)
	filesBefore := ls(t, ctx, fromDir)
	if !containsFile(filesBefore, filenameFrom) {
		fatalf(t, ctx, "%v does not contain %v before logrotate.", fromDir, filenameFrom)
	}
	ctx.log.Debugf("file list before logrotate: %#v", filenames(ls(t, ctx, fromDir)))
	switch {
	case ctx.logrotateCfg == _nocreate:
		moveOrFail(t, ctx, from, to)
	case ctx.logrotateCfg == _create:
		moveOrFail(t, ctx, from, to)
		ctx.log.Debugf("create %v", from)
		createOrFail(t, ctx, from)
	case ctx.logrotateCfg == _create_from_temp:
		moveOrFail(t, ctx, from, to)
		ctx.log.Debugf("create from temp %v", from)
		createFromTemp(t, ctx, from)
	case ctx.logrotateCfg == _copytruncate:
		if ctx.logrotateMvCfg != none {
			fatalf(t, ctx, "Rotating with '%v' does not make sense when moving the logfile with '%v'", ctx.logrotateCfg, ctx.logrotateMvCfg)
		}
		ctx.log.Debugf("cp %v %v", from, to)
		cpOrFail(t, ctx, from, to)
		ctx.log.Debugf("truncate %v", from)
		truncateOrFail(t, ctx, from)
	case ctx.logrotateCfg == _copy:
		if ctx.logrotateMvCfg != none {
			fatalf(t, ctx, "Rotating with '%v' does not make sense when moving the logfile with '%v'", ctx.logrotateCfg, ctx.logrotateMvCfg)
		}
		ctx.log.Debugf("cp %v %v", from, to)
		cpOrFail(t, ctx, from, to)
	default:
		fatalf(t, ctx, "Unknown logrotate option.")
	}
	ctx.log.Debugf("file list after logrotate: %#v", filenames(ls(t, ctx, fromDir)))
}

func ls(t *testing.T, ctx *context, path string) []os.FileInfo {
	result, err := ioutil.ReadDir(path)
	if err != nil {
		fatalf(t, ctx, "%v: Failed to list directory: %v", path, err.Error())
	}
	return result
}

func containsFile(files []os.FileInfo, filename string) bool {
	for _, f := range files {
		if filepath.Base(f.Name()) == filepath.Base(filename) {
			return true
		}
	}
	return false
}

func moveOrFail(t *testing.T, ctx *context, from, to string) {
	fromPath := path.Join(ctx.basedir, from)
	fromDir := filepath.Dir(fromPath)
	fromFilename := filepath.Base(fromPath)
	switch {
	case ctx.logrotateMvCfg == mv:
		ctx.log.Debugf("mv %v %v", from, to)
		mvOrFail(t, ctx, from, to)
	case ctx.logrotateMvCfg == cp:
		ctx.log.Debugf("cp %v %v", from, to)
		cpOrFail(t, ctx, from, to)
		ctx.log.Debugf("rm %v", from)
		rmOrFail(t, ctx, from)
	case ctx.logrotateMvCfg == rm:
		ctx.log.Debugf("rm %v", from)
		rmOrFail(t, ctx, from)
	}
	filesAfterRename := ls(t, ctx, fromDir)
	if containsFile(filesAfterRename, fromFilename) {
		fatalf(t, ctx, "%v still contains file %v after mv.", fromDir, fromFilename)
	}
}

func filenames(fileInfos []os.FileInfo) []string {
	result := make([]string, 0, len(fileInfos))
	for _, fileInfo := range fileInfos {
		result = append(result, fileInfo.Name())
	}
	return result
}

func mvOrFail(t *testing.T, ctx *context, from, to string) {
	fromPath := path.Join(ctx.basedir, from)
	toPath := path.Join(ctx.basedir, to)
	err := os.Rename(fromPath, toPath)
	if err != nil {
		fatalf(t, ctx, "%v: Failed to mv file: %v", fromPath, err.Error())
	}
}

func cpOrFail(t *testing.T, ctx *context, from, to string) {
	fromPath := path.Join(ctx.basedir, from)
	toPath := path.Join(ctx.basedir, to)
	data, err := ioutil.ReadFile(fromPath)
	if err != nil {
		fatalf(t, ctx, "%v: Copy failed, cannot read file: %v", fromPath, err.Error())
	}
	err = ioutil.WriteFile(toPath, data, 0644)
	if err != nil {
		fatalf(t, ctx, "%v: Copy failed, cannot write file: %v", toPath, err.Error())
	}
}

func rmOrFail(t *testing.T, ctx *context, from string) {
	fromPath := path.Join(ctx.basedir, from)
	err := os.Remove(fromPath)
	if err != nil {
		fatalf(t, ctx, "%v: Remove failed: %v", fromPath, err.Error())
	}
}

func createOrFail(t *testing.T, ctx *context, from string) {
	fromPath := path.Join(ctx.basedir, from)
	dir := filepath.Dir(fromPath)
	filename := filepath.Base(fromPath)
	filesBeforeCreate := ls(t, ctx, dir)
	if containsFile(filesBeforeCreate, filename) {
		fatalf(t, ctx, "%v contains file %v before create.", dir, filename)
	}
	f, err := os.Create(fromPath)
	if err != nil {
		fatalf(t, ctx, "Failed to re-create %v while simulating logrotate: %v", from, err.Error())
	}
	err = f.Close()
	if err != nil {
		fatalf(t, ctx, "%v: Failed to close file: %v", filename, err.Error())
	}
	filesAfterCreate := ls(t, ctx, dir)
	if !containsFile(filesAfterCreate, filename) {
		fatalf(t, ctx, "%v does not contain %v after create.", dir, filename)
	}
}

func createFromTemp(t *testing.T, ctx *context, from string) {
	fromPath := path.Join(ctx.basedir, from)
	dir := filepath.Dir(fromPath)
	filename := filepath.Base(fromPath)
	filesBeforeCreate := ls(t, ctx, dir)
	if containsFile(filesBeforeCreate, filename) {
		fatalf(t, ctx, "%v contains file %v before create.", dir, filename)
	}
	tmpFile, err := ioutil.TempFile(dir, "logrotate_temp.")
	if err != nil {
		fatalf(t, ctx, "failed to create temporary log file in %v: %v", dir, err.Error())
	}
	tmpFilename := tmpFile.Name()
	err = tmpFile.Close()
	if err != nil {
		fatalf(t, ctx, "failed to close temporary log file %v: %v", tmpFile.Name(), err.Error())
	}
	err = os.Rename(tmpFilename, fromPath)
	if err != nil {
		fatalf(t, ctx, "Failed to mv \"%v\" \"%v\": %v", tmpFilename, from, err.Error())
	}
	filesAfterCreate := ls(t, ctx, dir)
	if !containsFile(filesAfterCreate, filename) {
		fatalf(t, ctx, "%v does not contain %v after create.", dir, filename)
	}
}

func truncateOrFail(t *testing.T, ctx *context, from string) {
	fromPath := path.Join(ctx.basedir, from)
	err := os.Truncate(fromPath, 0)
	if err != nil {
		fatalf(t, ctx, "%v: Error truncating the file: %v", from, err.Error())
	}
}

func mkdir(t *testing.T, ctx *context, dirname string) {
	var (
		fullpath string
		err      error
	)
	fullpath = path.Join(ctx.basedir, dirname)
	if _, err = os.Stat(fullpath); !os.IsNotExist(err) {
		fatalf(t, ctx, "mkdir %v failed: directory already exists", dirname)
	}
	err = os.Mkdir(fullpath, 0755)
	if err != nil {
		fatalf(t, ctx, "mkdir %v failed: %v", dirname, err)
	}
}

func startFileTailer(t *testing.T, ctx *context, params []string) {
	var (
		parsedGlobs       []glob.Glob
		tailer            fswatcher.FileTailer
		readall           = false
		failOnMissingFile = true
		globs             []string
		err               error
	)
	for _, p := range params {
		switch p {
		case "readall=true":
			readall = true
		case "readall=false":
			readall = false
		case "fail_on_missing_logfile=true":
			failOnMissingFile = true
		case "fail_on_missing_logfile=false":
			failOnMissingFile = false
		default:
			globs = append(globs, p)
		}
	}
	for _, g := range globs {
		parsedGlob, err := glob.Parse(filepath.Join(ctx.basedir, g))
		if err != nil {
			fatalf(t, ctx, "%v", err)
		}
		parsedGlobs = append(parsedGlobs, parsedGlob)
	}
	if ctx.tailerCfg == fseventTailer {
		tailer, err = fswatcher.RunFileTailer(parsedGlobs, readall, failOnMissingFile, ctx.log)
	} else {
		tailer, err = fswatcher.RunPollingFileTailer(parsedGlobs, readall, failOnMissingFile, 10*time.Millisecond, ctx.log)
	}
	if err != nil {
		fatalf(t, ctx, "%v", err)
	}
	tailer = BufferedTailer(tailer)
	ctx.tailer = tailer
	ctx.linesFromTailer = makeLinesFromTailer(tailer)
}

func expect(t *testing.T, ctx *context, line string, file string) {
	actualLine, err := ctx.linesFromTailer.nextLine(filepath.Join(ctx.basedir, file), 500*time.Millisecond)
	if err != nil {
		fatalf(t, ctx, "%v: failed to read line %q: %v", file, line, err)
	}
	if line != actualLine {
		fatalf(t, ctx, "%v: expected line %q but got line %q", file, line, actualLine)
	}
}

func fatalf(t *testing.T, ctx *context, format string, args ...interface{}) {
	ctx.log.Errorf(format, args...) // Don't use ctx.log.Fatalf() here because this calls logger.Exit()
	t.Fatalf(format, args...)
}

type logrotateConfig int
type logrotateMoveConfig int
type loggerConfig int
type fileTailerConfig int

const ( // see 'man logrotate'
	_copy             logrotateConfig = iota // Don’t change the original logfile at all.
	_copytruncate                            // Truncate the original log file in place instead of removing it.
	_nocreate                                // Don't create a new logfile after rotation.
	_create                                  // Create a new empty logfile immediately after rotation.
	_create_from_temp                        // Like _create, but instead of creating the new logfile directly, logrotate creates an empty tempfile and then moves it to the logfile (see https://github.com/fstab/grok_exporter/pull/21)
)

const (
	mv   logrotateMoveConfig = iota // Move the old logfile to the backup.
	cp                              // Copy the old logfile to the backup, then remove it.
	rm                              // Delete the old logfile without keeping a backup.
	none                            // Do nothing, to be used in combination with _copytruncate and _copy
)

const (
	closeFileAfterEachLine loggerConfig = iota // Logger does not keep the file open.
	keepOpen                                   // Logger keeps the file open.
)

const (
	fseventTailer fileTailerConfig = iota
	pollingTailer
)

func (opt logrotateConfig) String() string {
	switch {
	case opt == _copy:
		return "copy"
	case opt == _copytruncate:
		return "copytruncate"
	case opt == _nocreate:
		return "nocreate"
	case opt == _create:
		return "create"
	case opt == _create_from_temp:
		return "create_from_temp"
	default:
		return "unknown"
	}
}

func (opt logrotateMoveConfig) String() string {
	switch {
	case opt == mv:
		return "mv"
	case opt == cp:
		return "cp"
	case opt == rm:
		return "rm"
	case opt == none:
		return "none"
	default:
		return "unknown"
	}
}

func (opt loggerConfig) String() string {
	switch {
	case opt == closeFileAfterEachLine:
		return "closeFileAfterEachLine"
	case opt == keepOpen:
		return "keepOpen"
	default:
		return "unknown"
	}
}

func (opt fileTailerConfig) String() string {
	switch {
	case opt == fseventTailer:
		return "fseventTailer"
	case opt == pollingTailer:
		return "pollingTailer"
	default:
		return "unknown"
	}
}

func mkTempDir(t *testing.T, ctx *context) string {
	dir, err := ioutil.TempDir("", "grok_exporter")
	if err != nil {
		fatalf(t, ctx, "Failed to create test directory: %v", err.Error())
	}
	return dir
}

func newLogFileWriter(t *testing.T, ctx *context, logfile string) logFileWriter {
	switch {
	case ctx.loggerCfg == closeFileAfterEachLine:
		return newCloseFileAfterEachLineLogFileWriter(t, logfile)
	case ctx.loggerCfg == keepOpen:
		return newKeepOpenLogFileWriter(t, ctx, logfile)
	default:
		fatalf(t, ctx, "%v: Unsupported logger config.", ctx.loggerCfg)
		return nil
	}
}

type logFileWriter interface {
	writeLine(t *testing.T, ctx *context, line string)
	close(t *testing.T, ctx *context)
}

type closeFileAfterEachLineLogFileWriter struct {
	path string
}

func newCloseFileAfterEachLineLogFileWriter(t *testing.T, logfile string) logFileWriter {
	return &closeFileAfterEachLineLogFileWriter{
		path: logfile,
	}
}

func (l *closeFileAfterEachLineLogFileWriter) writeLine(t *testing.T, ctx *context, line string) {
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fatalf(t, ctx, "%v: Failed to open file for writing: %v", l.path, err.Error())
	}
	newline := "\n"
	if runtime.GOOS == "windows" {
		newline = "\r\n"
	}
	_, err = f.WriteString(fmt.Sprintf("%v%v", line, newline))
	if err != nil {
		fatalf(t, ctx, "%v: Failed to write to file: %v", l.path, err.Error())
	}
	err = f.Sync()
	if err != nil {
		fatalf(t, ctx, "%v: Failed to flush file: %v", l.path, err.Error())
	}
	err = f.Close()
	if err != nil {
		fatalf(t, ctx, "%v: Failed to close file: %v", l.path, err.Error())
	}
	ctx.log.Debugf("Wrote log line '%v' with closeFileAfterEachLineLogger.", line)
}

func (l *closeFileAfterEachLineLogFileWriter) close(t *testing.T, ctx *context) {
	// nothing to do
}

type keepOpenLogFileWriter struct {
	file *os.File
}

func newKeepOpenLogFileWriter(t *testing.T, ctx *context, logfile string) logFileWriter {
	f, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fatalf(t, ctx, "%v: Failed to open file for writing: %v", logfile, err.Error())
	}
	return &keepOpenLogFileWriter{
		file: f,
	}
}

func (l *keepOpenLogFileWriter) writeLine(t *testing.T, ctx *context, line string) {
	_, err := l.file.WriteString(fmt.Sprintf("%v\n", line))
	if err != nil {
		fatalf(t, ctx, "%v: Failed to write to file: %v", l.file.Name(), err.Error())
	}
	err = l.file.Sync()
	if err != nil {
		fatalf(t, ctx, "%v: Failed to flush the file: %v", l.file.Name(), err.Error())
	}
	ctx.log.Debugf("Wrote log line '%v' with keepOpenLogger.", line)
}

func (l *keepOpenLogFileWriter) close(t *testing.T, ctx *context) {
	err := l.file.Close()
	if err != nil {
		fatalf(t, ctx, "%v: Failed to close logfile: %v", l.file.Name(), err.Error())
	}
}

func tearDown(t *testing.T, ctx *context) {
	deleteRecursively(t, ctx, ctx.basedir)
}

// Verbose implementation of os.RemoveAll() to debug a Windows "Access is denied" issue.
func deleteRecursively(t *testing.T, ctx *context, file string) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		fatalf(t, ctx, "tearDown: stat(%q) failed: %v", file, err)
	}
	if fileInfo.IsDir() {
		for _, childInfo := range ls(t, ctx, file) {
			deleteRecursively(t, ctx, path.Join(file, childInfo.Name()))
		}
	}
	ctx.log.Debugf("tearDown: removing %q", file)
	delete(t, ctx, file)
}

// Verbose implementation of os.Remove() to debug a Windows "Access is denied" issue.
func delete(t *testing.T, ctx *context, file string) {
	var (
		err, statErr error
		timeout      = 5 * time.Second
		timePassed   = 0 * time.Second
	)
	// Repeat a few times to ensure the Windows issue is not caused by a slow file tailer shutdown.
	// It's unlikely though, as assertGoroutinesTerminated() should make sure that the tailer is really terminated.
	for timePassed < timeout {
		err = os.Remove(file) // removes files and empty directories
		if err == nil {
			// Check if the file or directory is really removed. It seems that on Windows, os.Remove() sometimes
			// returns no error while the file or directory is still there.
			_, statErr = os.Stat(file)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					// os.Remove(file) was successful, the file or directory is gone.
					return
				} else {
					err = fmt.Errorf("stat failed: %v", statErr)
				}
			}
		}
		// os.Stat() successful. The file or directory is still there. Try again.
		time.Sleep(200 * time.Millisecond)
		timePassed += 200 * time.Millisecond
		if runtime.GOOS == "windows" {
			// On Windows, removing a watched directory fails with "Access is denied".
			// We ignore this here and move on. grok_exporter will never shut down the tailer but
			// keep it running until the application terminates, so this should not be a problem.
			return
		}
	}
	if err != nil {
		fatalf(t, ctx, "tearDown: %q: failed to remove file or directory: %v", file, err)
	} else {
		fatalf(t, ctx, "tearDown: %q: failed to remove file or directory", file)
	}
}

func printCmd(cmd []string) string {
	quoted := make([]string, 0, len(cmd))
	for i, arg := range cmd {
		if i > 0 && strings.Contains(arg, " ") {
			quoted = append(quoted, "'"+arg+"'")
		} else {
			quoted = append(quoted, arg)
		}
	}
	return strings.Join(quoted, " ")
}

func join(arr []fmt.Stringer, sep string) string {
	stringArr := make([]string, 0, len(arr))
	for _, a := range arr {
		stringArr = append(stringArr, a.String())
	}
	return strings.Join(stringArr, sep)
}

//func TestStress(t *testing.T) {
//	for i := 0; i < 250; i++ {
//		TestFileTailerCloseLogfileAfterEachLine(t)
//		TestFileTailerKeepLogfileOpen(t)
//	}
//}

func TestShutdownDuringSyscall(t *testing.T) {
	runTestShutdown(t, "reading")
}

func TestShutdownDuringSendLine(t *testing.T) {
	runTestShutdown(t, "writing")
}

func runTestShutdown(t *testing.T, mode string) {

	if runtime.GOOS == "windows" {
		t.Skip("The shutdown tests are flaky on Windows. We skip them until either golang.org/x/exp/winfsnotify is fixed, or until we do our own implementation. This shouldn't be a problem when running grok_exporter, because in grok_exporter the file system watcher is never stopped.")
		return
	}

	nGoroutinesBefore := runtime.NumGoroutine()

	ctx := setUp(t, "test shutdown while "+mode, closeFileAfterEachLine, fseventTailer, _nocreate, mv)
	writer := newLogFileWriter(t, ctx, path.Join(ctx.basedir, "test.log"))
	writer.writeLine(t, ctx, "line 1")

	parsedGlob, err := glob.Parse(filepath.Join(ctx.basedir, "test.log"))
	if err != nil {
		fatalf(t, ctx, "%q: failed to parse glob: %q", parsedGlob, err)
	}
	tailer, err := fswatcher.RunFileTailer([]glob.Glob{parsedGlob}, false, true, ctx.log)
	if err != nil {
		fatalf(t, ctx, "failed to start tailer: %v", err)
	}

	switch {
	case mode == "reading":
		// shutdown while the watcher is hanging in the blocking kevent() or syscall.Read() call
		time.Sleep(200 * time.Millisecond)
		tailer.Close()
	case mode == "writing":
		// shutdown while the watcher is sending an event
		writer.writeLine(t, ctx, "line 2")
		time.Sleep(200 * time.Millisecond)
		// tailer is now trying to write the line to the Lines channel, but we are not reading it
		tailer.Close()
	default:
		fatalf(t, ctx, "unknown mode: %v", mode)
	}
	select {
	case _, open := <-tailer.Errors():
		if open {
			fatalf(t, ctx, "error channel not closed")
		}
	case <-time.After(5 * time.Second):
		fatalf(t, ctx, "timeout while waiting for errors channel to be closed.")
	}
	select {
	case _, open := <-tailer.Lines():
		if open {
			fatalf(t, ctx, "lines channel not closed")
		}
	case <-time.After(5 * time.Second):
		fatalf(t, ctx, "timeout while waiting for errors channel to be closed.")
	}
	assertGoroutinesTerminated(t, ctx, nGoroutinesBefore)
}

func makeLinesFromTailer(tailer fswatcher.FileTailer) *linesFromTailer {
	return &linesFromTailer{
		tailer: tailer,
		buf:    make(map[string][]string),
	}
}

// Wrapper around FileTailer to get the next lines for a specific file.
type linesFromTailer struct {
	tailer fswatcher.FileTailer
	buf    map[string][]string
}

// Reads the next line for a specific file.
// If the tailer produces lines for other files they are buffered.
// This call may take longer than timeout if we keep reading lines from another file
func (l *linesFromTailer) nextLine(file string, timeout time.Duration) (string, error) {
	if len(l.buf[file]) > 0 {
		result := l.buf[file][0]
		l.buf[file] = l.buf[file][1:]
		return result, nil
	} else {
		for {
			select {
			case line := <-l.tailer.Lines():
				if line.File == file {
					return line.Line, nil
				} else {
					l.buf[line.File] = append(l.buf[line.File], line.Line)
				}
			case err := <-l.tailer.Errors():
				return "", err
			case <-time.After(5 * time.Second):
				return "", fmt.Errorf("timeout after %v", timeout)
			}
		}
	}
}
