/*
Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tika

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func init() {
	// Overwrite the cmder to inject a dummy command. We simulate starting a server
	// by running the TestHelperProcess.
	command = func(string, ...string) *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", "2")
		c.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return c
	}
}

func TestNewServerError(t *testing.T) {
	tests := []struct {
		name string
		jar  string
		port string
	}{
		{name: "no jar path"},
		{name: "invalid port", jar: "test_resources/test.jar", port: "%31"},
		{name: "missing jar file", jar: "test_resources/missing.jar"},
	}
	for _, test := range tests {
		if _, err := NewServer(test.jar, test.port); err == nil {
			t.Errorf("NewServer(%s) got no error", test.name)
		}
	}
}

func TestStart(t *testing.T) {
	path, err := os.Executable() // Use the text executable path as a dummy jar.
	if err != nil {
		t.Skip("cannot find current test executable")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "1.14")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("error creating test server: %v", err)
	}

	s, err := NewServer(path, tsURL.Port())
	if err != nil {
		t.Fatalf("NewServer got error: %v", err)
	}
	err = s.Start(context.Background())
	if err != nil {
		t.Fatalf("Start got error: %v", err)
	}
	s.Stop()
}

func bouncyServer(bounce int) *httptest.Server {
	bounced := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if bounced < bounce {
			bounced++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "1.14")
	}))

}

func TestStartError(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Skip("cannot find current test executable")
	}
	ts := bouncyServer(4)
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("error creating test server: %v", err)
	}
	s, err := NewServer(path, tsURL.Port())
	if err != nil {
		t.Fatalf("NewServer got error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Start(ctx); err == nil {
		t.Fatalf("s.Start got no error, want error")
	}
	s.jar = "nonexistentFile.jar"
	if err := s.Start(ctx); err == nil {
		t.Fatalf("s.Start got no error, want error for missing Jar file")
	}
}

func TestURL(t *testing.T) {
	tests := []string{"", "test"}
	for _, test := range tests {
		s := &Server{url: test}
		if got := s.URL(); got != test {
			t.Errorf("URL() = %q, want %q", got, test)
		}
	}
}

func TestWaitForStart(t *testing.T) {
	tests := []struct {
		name        string
		reqToBounce int
		wantError   bool
		timeout     time.Duration
	}{
		{"not bounced", 0, false, 5 * time.Second},
		{"bounced twice", 2, false, 5 * time.Second},
		{"bounced for too long", 4, true, 2 * time.Second},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ts := bouncyServer(test.reqToBounce)
			defer ts.Close()
			s := &Server{url: ts.URL}
			ctx, cancel := context.WithTimeout(context.Background(), test.timeout)
			defer cancel()
			got := s.waitForStart(ctx)
			if test.wantError && got == nil {
				t.Errorf("waitForStart(%s) got no error, want error", test.name)
			}
			if test.wantError {
				ts.Close()
				return
			}
			if got != nil {
				t.Errorf("waitForStart(%s) got %v, want no error", test.name, got)
			}
		})
	}
}

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun.
// Adapted from os/exec/exec_test.go.
func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if args[0] == "sleep" {
		l, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(1)
		}
		time.Sleep(time.Duration(l) * time.Second)
	}
	if args[0] == "exit" {
		code, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(1)
		}
		os.Exit(code)
	}
}

func TestInterruptExit(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{0, false},
		{1, false},
		{130, true},
	}
	for _, test := range tests {
		c := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "exit", strconv.Itoa(test.code))
		c.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		if got := interruptExit(c.Run()); got != test.want {
			t.Errorf("interruptExit(exit status %d) got %v, want %v", test.code, got, test.want)
		}
	}
}

func TestValidateFileHash(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Skip("cannot find current test executable")
	}

	tests := []struct {
		path    string
		wantErr bool
	}{
		{"path_to_non_existent_file", true},
		{path, false},
	}
	for _, test := range tests {
		_, err := sha512Hash(test.path)
		if test.wantErr && err == nil {
			t.Errorf("getHash(%s) wanted an error", test.path)
			continue
		}
		if !test.wantErr && err != nil {
			t.Errorf("getHash(%s) got an error: %v", test.path, err)
		}
	}
}

func TestDownloadServerError(t *testing.T) {
	tests := []struct {
		version Version
		path    string
	}{
		{"1.0", ""},
		{Version410, ""},
	}
	for _, test := range tests {
		if err := DownloadServer(context.Background(), test.version, test.path); err == nil {
			t.Errorf("DownloadServer(%q, %q) got no error, want an error", test.version, test.path)
		}
	}
}

func TestAddJavaProps(t *testing.T) {
	oldCommand := command
	defer func() { command = oldCommand }()

	path, err := os.Executable() // Use the text executable path as a dummy jar.
	if err != nil {
		t.Skip("cannot find current test executable")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "1.14")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("error creating test server: %v", err)
	}

	s, err := NewServer(path, tsURL.Port())
	if err != nil {
		t.Fatalf("NewServer got error: %v", err)
	}

	wantKey := "java.io.tmpdir"
	wantVal := "/tmp/stuff"
	s.JavaProps[wantKey] = wantVal

	command = func(c string, args ...string) *exec.Cmd {
		found := false
		want := fmt.Sprintf("-D%s=%q", wantKey, wantVal)
		for _, arg := range args {
			if arg == want {
				found = true
			}
		}
		if !found {
			t.Errorf("NewServer got %v %v args, want to contain %s", c, args, want)
		}
		return oldCommand(c, args...)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Errorf("Start got error: %v", err)
	}
}

func TestDownloadURL(t *testing.T) {
	tests := []struct {
		version Version
		want    string
	}{
		{Version121, "https://repo1.maven.org/maven2/org/apache/tika/tika-server/1.21/tika-server-1.21.jar"},
		{Version260, "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/2.6.0/tika-server-standard-2.6.0.jar"},
		{Version332, "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/3.3.2/tika-server-standard-3.3.2.jar"},
		{Version410, "https://archive.apache.org/dist/tika/4.1.0/tika-server-standard-4.1.0.zip"},
	}
	for _, test := range tests {
		if got := downloadURL(test.version); got != test.want {
			t.Errorf("downloadURL(%q) got %q, want %q", test.version, got, test.want)
		}
	}
}

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{"1.21", 1},
		{"3.3.2", 3},
		{"4.1.0", 4},
		{"4.2.0-SNAPSHOT", 4},
		{"", 0},
		{"unknown", 0},
	}
	for _, test := range tests {
		if got := majorVersion(test.version); got != test.want {
			t.Errorf("majorVersion(%q) got %d, want %d", test.version, got, test.want)
		}
	}
}

// writeZip writes a zip file at path containing files, a map from file name
// to contents. Names ending in "/" are written as directories.
func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// setSHA512 sets the sha512 for v to the hash of the file at path for the
// duration of the test.
func setSHA512(t *testing.T, v Version, path string) {
	t.Helper()
	hash, err := sha512Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	old := sha512s[v]
	sha512s[v] = hash
	t.Cleanup(func() { sha512s[v] = old })
}

func TestDownloadServerDirZip(t *testing.T) {
	dir := t.TempDir()
	// A zip that is already in dir and has the correct sha512 is not downloaded.
	writeZip(t, filepath.Join(dir, "tika-server-standard-4.1.0.zip"), map[string]string{
		"bin/":                           "",
		"lib/":                           "",
		"lib/tika-core-4.1.0.jar":        "tika core",
		"tika-server-standard-4.1.0.jar": "server",
	})
	setSHA512(t, Version410, filepath.Join(dir, "tika-server-standard-4.1.0.zip"))

	wantJar := filepath.Join(dir, "tika-server-standard-4.1.0.jar")
	wantFiles := map[string]string{
		wantJar: "server",
		filepath.Join(dir, "lib", "tika-core-4.1.0.jar"): "tika core",
	}
	checkFiles := func() {
		t.Helper()
		for path, want := range wantFiles {
			got, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("error reading extracted file: %v", err)
				continue
			}
			if string(got) != want {
				t.Errorf("extracted file %s got contents %q, want %q", path, got, want)
			}
		}
	}

	jar, err := DownloadServerDir(context.Background(), Version410, dir)
	if err != nil {
		t.Fatalf("DownloadServerDir got error: %v", err)
	}
	if jar != wantJar {
		t.Errorf("DownloadServerDir got %q, want %q", jar, wantJar)
	}
	checkFiles()
	if fi, err := os.Stat(filepath.Join(dir, "bin")); err != nil || !fi.IsDir() {
		t.Errorf("DownloadServerDir did not extract the bin directory: %v", err)
	}

	// A missing JAR means extraction did not complete, so the zip is extracted again.
	for path := range wantFiles {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := DownloadServerDir(context.Background(), Version410, dir); err != nil {
		t.Fatalf("DownloadServerDir got error: %v", err)
	}
	checkFiles()
}

func TestDownloadServerDirError(t *testing.T) {
	tests := []Version{"1.0", Version121, Version332}
	for _, v := range tests {
		dir := filepath.Join(t.TempDir(), "server")
		if _, err := DownloadServerDir(context.Background(), v, dir); err == nil {
			t.Errorf("DownloadServerDir(%q) got no error, want an error", v)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("DownloadServerDir(%q) created %s, want no directory", v, dir)
		}
	}
}

func TestUnzipInvalidPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "invalid.zip")
	writeZip(t, src, map[string]string{"../outside.txt": "outside"})

	dst := filepath.Join(dir, "dst")
	if err := unzip(src, dst, ""); err == nil {
		t.Error("unzip got no error, want an error for a path outside the directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.txt")); !os.IsNotExist(err) {
		t.Errorf("unzip wrote a file outside the directory: %v", err)
	}
}

func TestVersionsHaveSHA512(t *testing.T) {
	isHex := regexp.MustCompile("^[0-9a-f]{128}$")
	for _, v := range append(Versions, Version119, Version120, Version121) {
		if !isHex.MatchString(sha512s[v]) {
			t.Errorf("sha512s[%q] = %q, want 128 lowercase hex characters", v, sha512s[v])
		}
	}
}

func TestConfigPath(t *testing.T) {
	oldCommand := command
	defer func() { command = oldCommand }()

	path, err := os.Executable() // Use the text executable path as a dummy jar.
	if err != nil {
		t.Skip("cannot find current test executable")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Apache Tika 3.3.2")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("error creating test server: %v", err)
	}

	tests := []struct {
		name       string
		configPath string
		want       []string
	}{
		{"no config", "", nil},
		{"config", "/tmp/tika-config.xml", []string{"-c", "/tmp/tika-config.xml"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := NewServer(path, tsURL.Port())
			if err != nil {
				t.Fatalf("NewServer got error: %v", err)
			}
			s.ConfigPath = test.configPath

			command = func(c string, args ...string) *exec.Cmd {
				var got []string
				for i, arg := range args {
					if arg == "-c" && i+1 < len(args) {
						got = args[i : i+2]
					}
				}
				if !reflect.DeepEqual(got, test.want) {
					t.Errorf("Start got %v %v args, want config args %v", c, args, test.want)
				}
				return oldCommand(c, args...)
			}

			if err := s.Start(context.Background()); err != nil {
				t.Errorf("Start got error: %v", err)
			}
		})
	}
}
