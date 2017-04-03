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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func init() {
	// Overwrite the cmder to inject a dummy command. We simulate starting a server
	// by running the TestHelperProcess.
	cmder = func(cmd string, args ...string) *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", "2")
		c.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return c
	}
}

func TestStartServer(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Skip("cannot find current test executable")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "1.14")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("error creating test server: %v", err)
	}
	tests := []struct {
		name   string
		config *ServerConfig
	}{
		{name: "no config"},
		{
			name: "basic config",
			config: &ServerConfig{
				Hostname: tsURL.Hostname(),
				Port:     tsURL.Port(),
			},
		},
	}
	for _, test := range tests {
		_, err = StartServer(path, test.config)
		if err != nil {
			t.Errorf("StatServer error: %v", err)
		}
	}
}

func bouncyServer(bounce int) *httptest.Server {
	bounced := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bounced < bounce {
			bounced++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "1.14")
	}))

}

func TestStartServerError(t *testing.T) {
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
	tests := []struct {
		name   string
		jar    string
		config *ServerConfig
	}{
		{name: "no jar path"},
		{
			name: "invalid jar path",
			jar:  "/invalid/jar/path.jar",
		},
		{
			name: "invalid hostname",
			jar:  path,
			config: &ServerConfig{
				Hostname: "192.168.0.%31",
			},
		},
		{
			name: "unresponsive server",
			jar:  path,
			config: &ServerConfig{
				Hostname: tsURL.Hostname(),
				Port:     tsURL.Port(),
				Timeout:  2 * time.Second,
			},
		},
	}
	for _, test := range tests {
		if _, err := StartServer(test.jar, test.config); err == nil {
			t.Errorf("StartServer(%s) got no error, want error", test.name)
		}
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
			s := &Server{url: ts.URL, timeout: test.timeout}
			got := s.waitForStart()
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

func TestClose(t *testing.T) {
	s := &Server{}
	if s.Close() == nil {
		t.Errorf("Close got no error, want error")
	}

	c := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", "2")
	c.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	s = &Server{cmd: c}
	if err := c.Start(); err != nil {
		t.Errorf("Start error: %v", err)
	}

	time.Sleep(1)
	if err := s.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}

	s.done = make(chan error, 1)
	s.done <- nil
	if err := s.Close(); err != nil {
		t.Errorf("Close error: %v", err)
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
	switch args[0] {
	case "sleep":
		l, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(1)
		}
		time.Sleep(time.Duration(l) * time.Second)
	}
}

func TestValidateFileMD5(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Skip("cannot find current test executable")
	}

	tests := []struct {
		path      string
		md5String string
		want      bool
	}{
		{
			"path_to_non_existent_file",
			"does not match",
			false,
		},
		{
			path,
			"does not match",
			false,
		},
	}
	for _, test := range tests {
		if got := validateFileMD5(test.path, test.md5String); got != test.want {
			t.Errorf("validateFileMD5(%s, %s) = %t, want %t", test.path, test.md5String, got, test.want)
		}
	}
}

func TestDownloadServerError(t *testing.T) {
	tests := []struct {
		version string
		path    string
	}{
		{"1.0", ""},
	}
	for _, test := range tests {
		if err := DownloadServer(test.version, test.path); err == nil {
			t.Errorf("DownloadServer(%s, %s) got no error, want an error", test.version, test.path)
		}
	}
}
