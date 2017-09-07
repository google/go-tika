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
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

// Server represents a Tika server. Create a new Server with NewServer,
// start it with StartServer, and shut it down with Close.
// There is no need to create a Server for an already running Tika Server
// since you can pass its URL directly to a Client.
type Server struct {
	jar      string
	url      string // url is derived from port and hostname.
	port     string
	hostname string
	cmd      *exec.Cmd
	done     chan error
	timeout  time.Duration
}

// URL returns the URL of this Server.
func (s *Server) URL() string {
	return s.url
}

// An Option can be passed to NewServer to configure the Server.
type Option func(*Server)

// WithHostname returns an Option to set the host of the Server.
func WithHostname(h string) Option {
	return func(s *Server) {
		s.hostname = h
	}
}

// WithPort returns an Option to set the port of the Server.
func WithPort(p string) Option {
	return func(s *Server) {
		s.port = p
	}
}

// WithStartupTimeout returns an Option to set the timeout for how long to wait
// for the Server to start.
func WithStartupTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.timeout = d
	}
}

type commander func(string, ...string) *exec.Cmd

// cmder is used to stub out *exec.Cmd for testing.
var cmder commander = exec.Command

// NewServer creates a new Server.
func NewServer(jar string, options ...Option) (*Server, error) {
	if jar == "" {
		return nil, fmt.Errorf("no jar file specified")
	}
	if _, err := os.Stat(jar); err != nil {
		return nil, fmt.Errorf("jar file not found: %s", jar)
	}
	s := &Server{
		jar:      jar,
		port:     "9998",
		timeout:  10 * time.Second,
		hostname: "localhost",
	}
	for _, o := range options {
		o(s)
	}
	urlString := "http://" + s.hostname + ":" + s.port
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %v", urlString, err)
	}
	s.url = u.String()
	return s, nil
}

// Start starts the given server. Start will start a new Java process. The
// caller must call Close() when finished with the Server.
func (s *Server) Start() error {
	s.cmd = cmder("java", "-jar", s.jar, "-p", s.port)
	done := make(chan error, 1)

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := s.cmd.Start(); err != nil {
		return err
	}
	go func() {
		done <- s.cmd.Wait()
	}()

	if err := s.waitForStart(); err != nil {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(stderr); err != nil {
			return fmt.Errorf("error reading stderr: %v", err)
		}
		return fmt.Errorf("%v: %v", err, buf.String())
	}
	return nil
}

// waitForServer waits until the given Server is responding to requests.
// waitForStart returns an error if the server does not respond within the timeout.
func (s Server) waitForStart() error {
	c := NewClient(nil, s.url)
	var err error
	for i := time.Duration(0); i < s.timeout; i += time.Second {
		if _, err = c.Version(context.Background()); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

// Close shuts the Server down, releasing resources. Callers are responsible
// for calling Close after calling Start.
func (s *Server) Close() error {
	if s.cmd == nil {
		return errors.New("Close called on invalid Server: did you call Start?")
	}
	select {
	case err := <-s.done:
		return err
	default:
		return s.cmd.Process.Kill()
	}
}

func validateFileMD5(path, wantH string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	return fmt.Sprintf("%x", h.Sum(nil)) == wantH
}

// A Version represents a Tika Server version.
type Version string

// Supported versions of Tika Server.
const (
	Version114 Version = "1.14"
)

// DownloadServer downloads and validates the given server version,
// saving it at path. DownloadServer returns an error if it could
// not be downloaded/validated. Valid values for the version are 1.14.
func DownloadServer(ctx context.Context, version Version, path string) error {
	md5s := map[Version]string{
		Version114: "39055fc71358d774b9da066f80b1141c",
	}
	wantH := md5s[version]
	if wantH == "" {
		return fmt.Errorf("unsupported Tika version: %s", version)
	}

	if _, err := os.Stat(path); err == nil {
		if validateFileMD5(path, wantH) {
			return nil
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer out.Close()

	url := fmt.Sprintf("http://search.maven.org/remotecontent?filepath=org/apache/tika/tika-server/%s/tika-server-%s.jar", version, version)
	resp, err := ctxhttp.Get(ctx, nil, url)
	if err != nil {
		return fmt.Errorf("unable to download %q: %v", url, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("error saving download: %v", err)
	}

	if !validateFileMD5(path, wantH) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("invalid md5: error removing %s: %v", path, err)
		}
		return fmt.Errorf("invalid md5")
	}
	return nil
}
