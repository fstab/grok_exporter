// Copyright 2016-2017 The grok_exporter Authors
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
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type logrotateOption int
type logrotateMoveOption int
type loggerOption int
type watcherType int

const ( // see 'man logrotate'
	_copy             logrotateOption = iota // Don’t change the original logfile at all.
	_copytruncate                            // Truncate the original log file in place instead of removing it.
	_nocreate                                // Don't create a new logfile after rotation.
	_create                                  // Create a new empty logfile immediately after rotation.
	_create_from_temp                        // Like _create, but instead of creating the new logfile directly, logrotate creates an empty tempfile and then moves it to the logfile (see https://github.com/fstab/grok_exporter/pull/21)
)

const (
	mv logrotateMoveOption = iota // Move the old logfile to the backup.
	cp                            // Copy the old logfile to the backup, then remove it.
	rm                            // Delete the old logfile without keeping a backup.
)

const (
	closeFileAfterEachLine loggerOption = iota // Logger does not keep the file open.
	keepOpen                                   // Logger keeps the file open.
)

const (
	fsevent watcherType = iota
	polling
)

func (opt logrotateOption) String() string {
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

func (opt logrotateMoveOption) String() string {
	switch {
	case opt == mv:
		return "mv"
	case opt == cp:
		return "cp"
	case opt == rm:
		return "rm"
	default:
		return "unknown"
	}
}

func (opt loggerOption) String() string {
	switch {
	case opt == closeFileAfterEachLine:
		return "closeFileAfterEachLine"
	case opt == keepOpen:
		return "keepOpen"
	default:
		return "unknown"
	}
}

func (opt watcherType) String() string {
	switch {
	case opt == fsevent:
		return "fsevent"
	case opt == polling:
		return "polling"
	default:
		return "unknown"
	}
}

func TestFileTailerCloseLogfileAfterEachLine(t *testing.T) {
	testRunNumber := 0 // Helps to figure out which debug message belongs to which test run.
	for _, watcherOpt := range []watcherType{fsevent, polling} {
		for _, logrotateOpt := range []logrotateOption{_create, _nocreate, _create_from_temp} {
			for _, mvOpt := range []logrotateMoveOption{mv, cp, rm} {
				testRunNumber++
				t.Run(fmt.Sprintf("[%v]", testRunNumber), func(t *testing.T) {
					testLogrotate(t, NewTestRunLogger(testRunNumber), watcherOpt, logrotateOpt, mvOpt, closeFileAfterEachLine)
				})
			}
		}
		for _, logrotateOpt := range []logrotateOption{_copy, _copytruncate} {
			// For logrotate options 'copy' and 'copytruncate', only the mvOpt 'cp' makes sense.
			testRunNumber++
			t.Run(fmt.Sprintf("[%v]", testRunNumber), func(t *testing.T) {
				testLogrotate(t, NewTestRunLogger(testRunNumber), watcherOpt, logrotateOpt, cp, closeFileAfterEachLine)
			})
		}
	}
}

func TestFileTailerKeepLogfileOpen(t *testing.T) {
	testRunNumber := 100
	// When the logger keeps the file open, only the logrotate options 'copy' and 'copytruncate' make sense.
	for _, watcherOpt := range []watcherType{fsevent, polling} {
		testLogrotate(t, NewTestRunLogger(testRunNumber), watcherOpt, _copy, cp, keepOpen) // 100, 102
		testRunNumber++
		testLogrotate(t, NewTestRunLogger(testRunNumber), watcherOpt, _copytruncate, cp, keepOpen) // 101, 103
		testRunNumber++
	}
}

// On Mac OS, we receive an additional NOTE_ATTRIB event for each change on the file unless
// the file is located a "hidden" directory. This is probably because Mac OS updates attributes
// used for showing the files in the Finder. This does not happen for files in hidden directories:
// * directories starting with a dot are hidden
// * directories with the xattr com.apple.FinderInfo (like everything in /tmp) are hidden
// In order to test this, we must create a log file somewhere outside of /tmp, so we use $HOME.
func TestVisibleInOSXFinder(t *testing.T) {
	log := NewTestRunLogger(200)
	usr, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}
	testDir, err := ioutil.TempDir(usr.HomeDir, "grok_exporter_test_dir_")
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err.Error())
	}
	defer cleanUp(t, testDir)
	logfile := mkTmpFileOrFail(t, testDir)
	logFileWriter := newLogFileWriter(t, logfile, closeFileAfterEachLine)
	defer logFileWriter.close(t)
	logFileWriter.writeLine(t, log, "test line 1")
	tail := RunFseventFileTailer(logfile, false, true, log)
	defer tail.Close()
	go func() {
		for err := range tail.Errors() {
			t.Fatalf("Tailer failed: %v", err.Error())
		}
	}()
	// This test runs the tailer with readAll=false, i.e. the tailer reads only lines that are added after the
	// tailer is started. In order to avoid race conditions, we give it a second before we write 'test line 2'.
	time.Sleep(1 * time.Second)
	logFileWriter.writeLine(t, log, "test line 2")
	expect(t, log, tail.Lines(), "test line 2", 1*time.Second)
	// On Mac OS, we get a delayed NOTE_ATTRIB event after we wrote 'test line 2'. Wait 5 seconds for this event.
	time.Sleep(5 * time.Second)
	logFileWriter.writeLine(t, log, "test line 3")
	// If we wrongly interpret NOTE_ATTRIB as truncate, we read 'test line 1' again. If we correctly ignore
	// NOTE_ATTRIB here, we will read 'test line 3'.
	expect(t, log, tail.Lines(), "test line 3", 1*time.Second)
}

// test the "fail_on_missing_logfile: false" configuration
func TestFileMissingOnStartup(t *testing.T) {
	const logfileName = "grok_exporter_test_logfile.log"
	log := NewTestRunLogger(300)
	tmpDir := mkTmpDirOrFail(t)
	defer cleanUp(t, tmpDir)
	var logfile = fmt.Sprintf("%s%c%s", tmpDir, os.PathSeparator, logfileName)
	tail := RunFseventFileTailer(logfile, true, false, log)
	defer tail.Close()

	// We don't expect errors. However, start a go-routine listening on
	// the tailer's errorChannel in case something goes wrong.
	go func() {
		for err := range tail.Errors() {
			t.Errorf("Tailer failed: %v", err.Error()) // Cannot call t.Fatalf() in other goroutine.
		}
	}()

	// Double check that file does not exist yet
	_, err := os.Stat(logfile)
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("%v should not exist yet")
	}

	logFileWriter := newLogFileWriter(t, logfile, closeFileAfterEachLine)
	logFileWriter.writeLine(t, log, "test line 1")
	logFileWriter.writeLine(t, log, "test line 2")
	expect(t, log, tail.Lines(), "test line 1", 1*time.Second)
	expect(t, log, tail.Lines(), "test line 2", 1*time.Second)
}

func TestShutdownDuringSyscall(t *testing.T) {
	runTestShutdown(t, "shutdown while the watcher is hanging in the blocking kevent() or syscall.Read() call")
}

func TestShutdownDuringSendEvent(t *testing.T) {
	runTestShutdown(t, "shutdown while the watcher is sending an event")
}

//func TestStress(t *testing.T) {
//	for i := 0; i < 250; i++ {
//		TestFileTailerCloseLogfileAfterEachLine(t)
//		TestFileTailerKeepLogfileOpen(t)
//	}
//}

func testLogrotate(t *testing.T, log simpleLogger, watcherOpt watcherType, logrotateOpt logrotateOption, logrotateMoveOpt logrotateMoveOption, loggerOpt loggerOption) {
	tmpDir := mkTmpDirOrFail(t)
	defer cleanUp(t, tmpDir)
	logfile := mkTmpFileOrFail(t, tmpDir)
	logFileWriter := newLogFileWriter(t, logfile, loggerOpt)
	defer logFileWriter.close(t)

	log.Debug("Running test using logfile %v with watcher option '%v', logrotate option '%v', move option '%v', and logger option '%v'.\n", path.Base(logfile), watcherOpt, logrotateOpt, logrotateMoveOpt, loggerOpt)

	logFileWriter.writeLine(t, log, "test line 1")
	logFileWriter.writeLine(t, log, "test line 2")

	var tail Tailer
	switch watcherOpt {
	case fsevent:
		tail = RunFseventFileTailer(logfile, true, true, log)
	case polling:
		tail = RunPollingFileTailer(logfile, true, true, 10*time.Millisecond, log)
	}
	defer tail.Close()

	// We don't expect errors. However, start a go-routine listening on
	// the tailer's errorChannel in case something goes wrong.
	go func() {
		for err := range tail.Errors() {
			t.Errorf("Tailer failed: %v", err.Error()) // Cannot call t.Fatalf() in other goroutine.
		}
	}()

	// The first two lines are received without any fsnotify event,
	// because they were written before the watcher was started.
	expect(t, log, tail.Lines(), "test line 1", 1*time.Second)
	expect(t, log, tail.Lines(), "test line 2", 1*time.Second)

	// Append a line and see if the event is processed.
	logFileWriter.writeLine(t, log, "test line 3")
	expect(t, log, tail.Lines(), "test line 3", 1*time.Second)

	rotate(t, log, logfile, logrotateOpt, logrotateMoveOpt)

	// Log two more lines and see if they are received.
	logFileWriter.writeLine(t, log, "line 4")
	expect(t, log, tail.Lines(), "line 4", 5*time.Second) // few seconds longer to get filesystem notifications for rotate()
	logFileWriter.writeLine(t, log, "line 5")
	expect(t, log, tail.Lines(), "line 5", 1*time.Second)
}

func newLogFileWriter(t *testing.T, logfile string, opt loggerOption) logFileWriter {
	switch {
	case opt == closeFileAfterEachLine:
		return newCloseFileAfterEachLineLogFileWriter(t, logfile)
	case opt == keepOpen:
		return newKeepOpenLogFileWriter(t, logfile)
	default:
		t.Fatalf("%v: Unsupported logger option.", opt)
		return nil
	}
}

type logFileWriter interface {
	writeLine(t *testing.T, log simpleLogger, line string)
	close(t *testing.T)
}

type closeFileAfterEachLineLogFileWriter struct {
	path string
}

func newCloseFileAfterEachLineLogFileWriter(t *testing.T, logfile string) logFileWriter {
	return &closeFileAfterEachLineLogFileWriter{
		path: logfile,
	}
}

func (l *closeFileAfterEachLineLogFileWriter) writeLine(t *testing.T, log simpleLogger, line string) {
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("%v: Failed to open file for writing: %v", l.path, err.Error())
	}
	_, err = f.WriteString(fmt.Sprintf("%v\n", line))
	if err != nil {
		t.Fatalf("%v: Failed to write to file: %v", l.path, err.Error())
	}
	err = f.Sync()
	if err != nil {
		t.Fatalf("%v: Failed to flush file: %v", l.path, err.Error())
	}
	err = f.Close()
	if err != nil {
		t.Fatalf("%v: Failed to close file: %v", l.path, err.Error())
	}
	log.Debug("Wrote log line '%v' with closeFileAfterEachLineLogger.\n", line)
}

func (l *closeFileAfterEachLineLogFileWriter) close(t *testing.T) {
	// nothing to do
}

type keepOpenLogFileWriter struct {
	file *os.File
}

func newKeepOpenLogFileWriter(t *testing.T, logfile string) logFileWriter {
	f, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("%v: Failed to open file for writing: %v", logfile, err.Error())
	}
	return &keepOpenLogFileWriter{
		file: f,
	}
}

func (l *keepOpenLogFileWriter) writeLine(t *testing.T, log simpleLogger, line string) {
	_, err := l.file.WriteString(fmt.Sprintf("%v\n", line))
	if err != nil {
		t.Fatalf("%v: Failed to write to file: %v", l.file.Name(), err.Error())
	}
	err = l.file.Sync()
	if err != nil {
		t.Fatalf("%v: Failed to flush the file: %v", l.file.Name(), err.Error())
	}
	log.Debug("Wrote log line '%v' with keepOpenLogger.\n", line)
}

func (l *keepOpenLogFileWriter) close(t *testing.T) {
	err := l.file.Close()
	if err != nil {
		t.Fatalf("%v: Failed to close logfile: %v", l.file.Name(), err.Error())
	}
}

func mkTmpDirOrFail(t *testing.T) string {
	testdir, err := ioutil.TempDir("", "grok_exporter")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err.Error())
	}
	return testdir
}

func cleanUp(t *testing.T, dir string) {
	// tailer.Close() on Windows is flaky: sometimes the tailer still reads files after Close() was called.
	// As a workaround, we don't delete the tmp dir on Windows in our tests.
	// This shouldn't be a problem when running grok_exporter in production, because in production the file system watcher runs forever and is never closed.
	if runtime.GOOS == "windows" {
		return
	}
	err := os.RemoveAll(dir)
	if err != nil {
		t.Fatalf("%v: Failed to remove the test directory after running the tests: %v", dir, err.Error())
	}
}

func mkTmpFileOrFail(t *testing.T, dir string) string {
	if len(ls(t, dir)) != 0 {
		t.Fatalf("%v: Directory not empty.", dir)
	}
	logfile, err := ioutil.TempFile(dir, "grok_exporter_test_log")
	if err != nil {
		t.Fatalf("Cannot create temp file: %v", err.Error())
	}
	err = logfile.Close()
	if err != nil {
		t.Fatalf("Failed to close temp file: %v", err.Error())
	}
	if len(ls(t, dir)) != 1 {
		t.Fatalf("%v: Expected exactly one file in directory.", dir)
	}
	return logfile.Name()
}

func containsFile(files []os.FileInfo, filename string) bool {
	for _, f := range files {
		if filepath.Base(f.Name()) == filepath.Base(filename) {
			return true
		}
	}
	return false
}

func ls(t *testing.T, path string) []os.FileInfo {
	result, err := ioutil.ReadDir(path)
	if err != nil {
		t.Fatalf("%v: Failed to list directory: %v", path, err.Error())
	}
	return result
}

func rotate(t *testing.T, log simpleLogger, logfile string, opt logrotateOption, mvOpt logrotateMoveOption) {
	dir := filepath.Dir(logfile)
	filename := filepath.Base(logfile)
	filesBefore := ls(t, dir)
	if !containsFile(filesBefore, filename) {
		t.Fatalf("%v does not contain %v before logrotate.", dir, filename)
	}
	switch {
	case opt == _nocreate:
		moveOrFail(t, mvOpt, logfile)
	case opt == _create:
		moveOrFail(t, mvOpt, logfile)
		createOrFail(t, logfile)
	case opt == _create_from_temp:
		moveOrFail(t, mvOpt, logfile)
		createFromTemp(t, logfile)
	case opt == _copytruncate:
		if mvOpt != cp {
			t.Fatalf("Rotating with '%v' does not make sense when moving the logfile with '%v'", opt, mvOpt)
		}
		cpOrFail(t, logfile, fmt.Sprintf("%v.1", logfile))
		truncateOrFail(t, logfile)
	case opt == _copy:
		if mvOpt != cp {
			t.Fatalf("Rotating with '%v' does not make sense when moving the logfile with '%v'", opt, mvOpt)
		}
		cpOrFail(t, logfile, fmt.Sprintf("%v.1", logfile))
	default:
		t.Fatalf("Unknown logrotate option.")
	}
	log.Debug("Simulated logrotate with option %v and mvOption %v\n", opt, mvOpt)
}

func moveOrFail(t *testing.T, mvOpt logrotateMoveOption, logfile string) {
	dir := filepath.Dir(logfile)
	filename := filepath.Base(logfile)
	switch {
	case mvOpt == mv:
		mvOrFail(t, logfile, fmt.Sprintf("%v.1", logfile))
	case mvOpt == cp:
		cpOrFail(t, logfile, fmt.Sprintf("%v.1", logfile))
		rmOrFail(t, logfile)
	case mvOpt == rm:
		rmOrFail(t, logfile)
	}
	filesAfterRename := ls(t, dir)
	if containsFile(filesAfterRename, filename) {
		t.Fatalf("%v still contains file %v after mv.", dir, filename)
	}
}

func mvOrFail(t *testing.T, fromPath string, toPath string) {
	err := os.Rename(fromPath, toPath)
	if err != nil {
		t.Fatalf("%v: Failed to mv file: %v", fromPath, err.Error())
	}
}

func cpOrFail(t *testing.T, fromPath string, toPath string) {
	data, err := ioutil.ReadFile(fromPath)
	if err != nil {
		t.Fatalf("%v: Copy failed, cannot read file: %v", fromPath, err.Error())
	}
	err = ioutil.WriteFile(toPath, data, 0644)
	if err != nil {
		t.Fatalf("%v: Copy failed, cannot write file: %v", toPath, err.Error())
	}
}

func rmOrFail(t *testing.T, path string) {
	err := os.Remove(path)
	if err != nil {
		t.Fatalf("%v: Remove failed: %v", path, err.Error())
	}
}

func createOrFail(t *testing.T, logfile string) {
	dir := filepath.Dir(logfile)
	filename := filepath.Base(logfile)
	filesBeforeCreate := ls(t, dir)
	if containsFile(filesBeforeCreate, filename) {
		t.Fatalf("%v contains file %v before create.", dir, filename)
	}
	f, err := os.Create(logfile)
	if err != nil {
		t.Fatalf("Failed to re-create %v while simulating logrotate: %v", filename, err.Error())
	}
	err = f.Close()
	if err != nil {
		t.Fatalf("%v: Failed to close file: %v", filename, err.Error())
	}
	filesAfterCreate := ls(t, dir)
	if !containsFile(filesAfterCreate, filename) {
		t.Fatalf("%v does not contain %v after create.", dir, filename)
	}
}

func createFromTemp(t *testing.T, logfile string) {
	dir := filepath.Dir(logfile)
	filename := filepath.Base(logfile)
	filesBeforeCreate := ls(t, dir)
	if containsFile(filesBeforeCreate, filename) {
		t.Fatalf("%v contains file %v before create.", dir, filename)
	}
	tmpFile, err := ioutil.TempFile(dir, "logrotate_temp.")
	if err != nil {
		t.Fatalf("failed to create temporary log file in %v: %v", dir, err.Error())
	}
	tmpFilename := tmpFile.Name()
	err = tmpFile.Close()
	if err != nil {
		t.Fatalf("failed to close temporary log file %v: %v", tmpFile.Name(), err.Error())
	}
	err = os.Rename(tmpFilename, logfile)
	if err != nil {
		t.Fatalf("Failed to mv \"%v\" \"%v\": %v", tmpFilename, logfile, err.Error())
	}
	filesAfterCreate := ls(t, dir)
	if !containsFile(filesAfterCreate, filename) {
		t.Fatalf("%v does not contain %v after create.", dir, filename)
	}
}

func truncateOrFail(t *testing.T, logfile string) {
	err := os.Truncate(logfile, 0)
	if err != nil {
		t.Fatalf("%v: Error truncating the file: %v", logfile, err.Error())
	}
}

func expect(t *testing.T, log simpleLogger, c chan string, line string, timeout time.Duration) {
	timeoutChan := make(chan bool, 1)
	go func() {
		time.Sleep(timeout)
		close(timeoutChan)
	}()
	select {
	case result := <-c:
		if result != line {
			t.Fatalf("Expected '%v', but got '%v'.", line, result)
		} else {
			log.Debug("Read expected line '%v'\n", line)
		}
	case <-timeoutChan:
		log.Debug("Timeout after %v while waiting for line '%v'\n", timeout, line)
		t.Fatalf("Timeout after %v while waiting for line '%v'", timeout, line)
	}
}

type testRunLogger struct {
	testRunNumber int
}

func NewTestRunLogger(testRunNumber int) *testRunLogger {
	return &testRunLogger{
		testRunNumber: testRunNumber,
	}
}

func (l *testRunLogger) Debug(format string, a ...interface{}) {
	fmt.Printf("%v [%v] %v", time.Now().Format("2006-01-02 15:04:05.0000"), l.testRunNumber, fmt.Sprintf(format, a...))
}

func runTestShutdown(t *testing.T, mode string) {

	if runtime.GOOS == "windows" {
		t.Skip("The shutdown tests are flaky on Windows. We skip them until either golang.org/x/exp/winfsnotify is fixed, or until we do our own implementation. This shouldn't be a problem when running grok_exporter, because in grok_exporter the file system watcher is never stopped.")
		return
	}

	tmpDir := mkTmpDirOrFail(t)
	defer cleanUp(t, tmpDir)

	logfile := mkTmpFileOrFail(t, tmpDir)
	file, err := open(logfile)
	if err != nil {
		t.Fatalf("Cannot create temp file: %v", err.Error())
	}
	defer file.Close()

	lines := make(chan string)
	defer close(lines)

	watcher, err := NewFseventWatcher(logfile, file)
	if err != nil {
		t.Fatalf("%v", err)
	}

	eventLoop := watcher.StartEventLoop()

	switch {
	case mode == "shutdown while the watcher is hanging in the blocking kevent() or syscall.Read() call":
		time.Sleep(200 * time.Millisecond)
		eventLoop.Close()
	case mode == "shutdown while the watcher is sending an event":
		file.Close()
		err = os.Remove(logfile) // trigger file system event so kevent() or syscall.Read() returns.
		if err != nil {
			t.Fatalf("Failed to remove logfile: %v", err)
		}
		// The watcher is now waiting until we read the event from the event channel.
		// However, we shut down and abort the event.
		eventLoop.Close()
	default:
		t.Fatalf("Unknown mode: %v", mode)
	}
	select {
	case _, ok := <-eventLoop.Errors():
		if ok {
			t.Fatalf("error channel not closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout while waiting for errors channel to be closed.")
	}
	select {
	case _, ok := <-eventLoop.Events():
		if ok {
			t.Fatalf("events channel not closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout while waiting for errors channel to be closed.")
	}
}
