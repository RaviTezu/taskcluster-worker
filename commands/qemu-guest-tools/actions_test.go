package qemuguesttools

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taskcluster/taskcluster-worker/engines"
	"github.com/taskcluster/taskcluster-worker/engines/qemu/metaservice"
	"github.com/taskcluster/taskcluster-worker/runtime"
	"github.com/taskcluster/taskcluster-worker/runtime/mocks"
)

func nilOrFatal(t *testing.T, err error, a ...interface{}) {
	if err != nil {
		t.Fatal(append(a, err)...)
	}
}

func assert(t *testing.T, condition bool, a ...interface{}) {
	if !condition {
		t.Fatal(a...)
	}
}

func TestGuestToolsProcessingActions(t *testing.T) {
	// Doesn't currently run on Windows, let's skip until Windows is a priority
	if goruntime.GOOS == "windows" {
		t.Skip("Skipping on Windows - when we start supporting Windows, we should reenable!")
	}
	// Create temporary storage
	storage, err := runtime.NewTemporaryStorage(os.TempDir())
	if err != nil {
		panic("Failed to create TemporaryStorage")
	}
	environment := &runtime.Environment{
		TemporaryStorage: storage,
	}

	logTask := bytes.NewBuffer(nil)
	meta := metaservice.New([]string{}, map[string]string{}, logTask, func(r bool) {
		panic("This test shouldn't get to this point!")
	}, environment)

	// Create http server for testing
	ts := httptest.NewServer(meta)
	defer ts.Close()
	defer meta.StopPollers() // Hack to stop pollers, otherwise server will block
	u, err := url.Parse(ts.URL)
	if err != nil {
		panic("Expected a url we can parse")
	}

	// Create an run guest-tools
	g := new(u.Host, mocks.NewMockMonitor(true))

	// start processing actions
	go g.ProcessActions()
	defer g.StopProcessingActions()

	////////////////////
	debug("### Test meta.GetArtifact")
	f, err := storage.NewFolder()
	if err != nil {
		panic("Failed to create temp folder")
	}
	defer f.Remove()

	testFile := filepath.Join(f.Path(), "hello.txt")
	err = ioutil.WriteFile(testFile, []byte("hello-world"), 0777)
	nilOrFatal(t, err, "Failed to create testFile: ", testFile)

	debug(" - request file: %s", testFile)
	r, err := meta.GetArtifact(testFile)
	nilOrFatal(t, err, "meta.GetArtifact failed, error: ", err)

	debug(" - reading testFile")
	data, err := ioutil.ReadAll(r)
	nilOrFatal(t, err, "Failed to read testFile")
	debug(" - read: '%s'", string(data))
	assert(t, string(data) == "hello-world", "Wrong payload: ", string(data))

	////////////////////
	debug("### Test meta.GetArtifact (missing file)")
	r, err = meta.GetArtifact(filepath.Join(f.Path(), "missing-file.txt"))
	assert(t, r == nil, "Expected error wihtout a reader")
	assert(t, err == engines.ErrResourceNotFound, "Expected ErrResourceNotFound")

	////////////////////
	debug("### Test meta.ListFolder")
	testFolder := filepath.Join(f.Path(), "test-folder")
	err = os.Mkdir(testFolder, 0777)
	nilOrFatal(t, err, "Failed to create test-folder/")

	testFile2 := filepath.Join(testFolder, "hello2.txt")
	err = ioutil.WriteFile(testFile2, []byte("hello-world-2"), 0777)
	nilOrFatal(t, err, "Failed to create testFile2: ", testFile2)

	debug(" - meta.ListFolder")
	files, err := meta.ListFolder(f.Path())
	nilOrFatal(t, err, "ListFolder failed, err: ", err)

	assert(t, len(files) == 2, "Expected 2 files")
	assert(t, files[0] == testFile || files[1] == testFile, "Expected testFile")
	assert(t, files[0] == testFile2 || files[1] == testFile2, "Expected testFile2")

	////////////////////
	debug("### Test meta.ListFolder (missing folder)")
	files, err = meta.ListFolder(filepath.Join(f.Path(), "no-such-folder"))
	assert(t, files == nil, "Expected files == nil, we hopefully have an error")
	assert(t, err == engines.ErrResourceNotFound, "Expected ErrResourceNotFound")

	////////////////////
	debug("### Test meta.ListFolder (empty folder)")
	emptyFolder := filepath.Join(f.Path(), "empty-folder")
	err = os.Mkdir(emptyFolder, 0777)
	nilOrFatal(t, err, "Failed to create empty-folder/")

	files, err = meta.ListFolder(emptyFolder)
	assert(t, len(files) == 0, "Expected zero files")
	assert(t, err == nil, "Didn't expect any error")

	////////////////////
	testShellHello(t, meta)
	testShellCat(t, meta)
	testShellCatStdErr(t, meta)
}

func testShellHello(t *testing.T, meta *metaservice.MetaService) {
	debug("### Test meta.Shell (using 'echo hello')")
	shell, err := meta.ExecShell(nil, false)
	nilOrFatal(t, err, "Failed to call meta.ExecShell()")

	readHello := sync.WaitGroup{}
	readHello.Add(1)
	// Discard stderr
	go io.Copy(ioutil.Discard, shell.StderrPipe())
	go func() {
		shell.StdinPipe().Write([]byte("echo HELLO\n"))
		readHello.Wait()
		shell.StdinPipe().Close()
	}()
	go func() {
		data := bytes.Buffer{}
		for {
			b := []byte{0}
			n, werr := shell.StdoutPipe().Read(b)
			data.Write(b[:n])
			if strings.Contains(data.String(), "HELLO") {
				readHello.Done()
				break
			}
			if werr != nil {
				assert(t, werr == io.EOF, "Expected EOF!")
				break
			}
		}
		// Discard the rest
		go io.Copy(ioutil.Discard, shell.StdoutPipe())
	}()

	success, err := shell.Wait()
	nilOrFatal(t, err, "Got an error from shell.Wait, error: ", err)
	assert(t, success, "Expected success from shell, we closed with end of stdin")
}

func testShellCat(t *testing.T, meta *metaservice.MetaService) {
	debug("### Test meta.Shell (using 'exec cat -')")
	shell, err := meta.ExecShell(nil, false)
	nilOrFatal(t, err, "Failed to call meta.ExecShell()")

	input := make([]byte, 42*1024*1024+7)
	rand.Read(input)

	// Discard stderr
	go io.Copy(ioutil.Discard, shell.StderrPipe())
	go func() {
		if goruntime.GOOS == "windows" {
			shell.StdinPipe().Write([]byte("type con\n"))
		} else {
			shell.StdinPipe().Write([]byte("exec cat -\n"))
		}
		// Give cat - some time to start, bash/busybox/dash won't work otherwise
		// Or they will work, but only intermittently!!!
		time.Sleep(250 * time.Millisecond)
		shell.StdinPipe().Write(input)
		shell.StdinPipe().Close()
		debug("Closed stdin")
	}()
	var output []byte
	outputDone := sync.WaitGroup{}
	outputDone.Add(1)
	go func() {
		data, rerr := ioutil.ReadAll(shell.StdoutPipe())
		nilOrFatal(t, rerr, "Got error from stdout pipe, error: ", rerr)
		output = data
		outputDone.Done()
	}()

	success, err := shell.Wait()
	nilOrFatal(t, err, "Got an error from shell.Wait, error: ", err)
	assert(t, success, "Expected success from shell, we closed with end of stdin")
	outputDone.Wait()
	assert(t, bytes.Equal(output, input), "Expected data to match input, ",
		"len(input) = ", len(input), " len(output) = ", len(output))
}

func testShellCatStdErr(t *testing.T, meta *metaservice.MetaService) {
	debug("### Test meta.Shell (using 'exec cat - 1>&2')")
	shell, err := meta.ExecShell(nil, false)
	nilOrFatal(t, err, "Failed to call meta.ExecShell()")

	input := make([]byte, 4*1024*1024+37)
	rand.Read(input)

	// Discard stderr
	go io.Copy(ioutil.Discard, shell.StdoutPipe())
	go func() {
		if goruntime.GOOS == "windows" {
			shell.StdinPipe().Write([]byte("type con 1>&2\n"))
		} else {
			shell.StdinPipe().Write([]byte("exec cat -  1>&2\n"))
		}
		// Give cat - some time to start, bash/busybox/dash won't work otherwise
		// Or they will work, but only intermittently!!!
		time.Sleep(250 * time.Millisecond)
		shell.StdinPipe().Write(input)
		shell.StdinPipe().Close()
		debug("Closed stdin")
	}()
	var output []byte
	outputDone := sync.WaitGroup{}
	outputDone.Add(1)
	go func() {
		data, rerr := ioutil.ReadAll(shell.StderrPipe())
		nilOrFatal(t, rerr, "Got error from stderr pipe, error: ", rerr)
		output = data
		outputDone.Done()
	}()

	success, err := shell.Wait()
	nilOrFatal(t, err, "Got an error from shell.Wait, error: ", err)
	assert(t, success, "Expected success from shell, we closed with end of stdin")
	outputDone.Wait()
	assert(t, bytes.Equal(output, input), "Expected data to match input, ",
		"len(input) = ", len(input), " len(output) = ", len(output))
}
