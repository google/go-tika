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
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

// Server represents a Tika server. Create a new Server with NewServer,
// start it with Start, and shut it down with the close function returned
// from Start.
// There is no need to create a Server for an already running Tika Server
// since you can pass its URL directly to a Client.
type Server struct {
	jar  string
	url  string // url is derived from port.
	port string
	cmd  *exec.Cmd
}

// URL returns the URL of this Server.
func (s *Server) URL() string {
	return s.url
}

// NewServer creates a new Server. The default port is 9998.
func NewServer(jar, port string) (*Server, error) {
	if jar == "" {
		return nil, fmt.Errorf("no jar file specified")
	}
	if port == "" {
		port = "9998"
	}
	s := &Server{
		jar:  jar,
		port: port,
	}
	urlString := "http://localhost:" + s.port
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %v", s.port, err)
	}
	s.url = u.String()
	return s, nil
}

var command = exec.Command

// Start starts the given server. Start will start a new Java process. The
// caller must call Stop() to shut down the process when finished with the
// Server. Start will wait for the server to be available or until ctx is
// cancelled.
func (s *Server) Start(ctx context.Context) error {
	cmd := command("java", "-jar", s.jar, "-p", s.port)

	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd

	if err := s.waitForStart(ctx); err != nil {
		out, readErr := cmd.CombinedOutput()
		if readErr != nil {
			return fmt.Errorf("error reading output: %v", readErr)
		}
		// Report stderr since sometimes the server says why it failed to start.
		return fmt.Errorf("error starting server: %v\nserver stderr:\n\n%s", err, out)
	}
	return nil
}

// waitForServer waits until the given Server is responding to requests or
// ctx is Done().
func (s Server) waitForStart(ctx context.Context) error {
	c := NewClient(nil, s.url)
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if _, err := c.Version(ctx); err == nil {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Stop shuts the server down, killing the underlying Java process. Stop
// must be called when finished with the server to avoid leaking the
// Java process. If s has not been started, Stop will panic.
func (s *Server) Stop() error {
	if err := s.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("could not kill server: %v", err)
	}
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("could not wait for server to finish: %v", err)
	}
	return nil
}

func md5Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// A version represents a Tika Server version.
type version string

// Supported versions of Tika Server.
const (
	Version114 version = "1.14"
	Version115 version = "1.15"
	Version116 version = "1.16"
)

var md5s = map[version]string{
	Version114: "39055fc71358d774b9da066f80b1141c",
	Version115: "80bd3f00f05326d5190466de27d593dd",
	Version116: "6a549ce6ef6e186e019766059fd82fb2",
}

// DownloadServer downloads and validates the given server version,
// saving it at path. DownloadServer returns an error if it could
// not be downloaded/validated. Valid values for the version are 1.14.
// It is the caller's responsibility to remove the file when no longer needed.
// If the file already exists and has the correct MD5, DownloadServer will
// do nothing.
func DownloadServer(ctx context.Context, v version, path string) error {
	hash := md5s[v]
	if hash == "" {
		return fmt.Errorf("unsupported Tika version: %s", v)
	}

	if got, err := md5Hash(path); err == nil {
		if got == hash {
			return nil
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer out.Close()

	url := fmt.Sprintf("http://search.maven.org/remotecontent?filepath=org/apache/tika/tika-server/%s/tika-server-%s.jar", v, v)
	resp, err := ctxhttp.Get(ctx, nil, url)
	if err != nil {
		return fmt.Errorf("unable to download %q: %v", url, err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("error saving download: %v", err)
	}

	h, err := md5Hash(path)

	if err != nil {
		return err
	}
	if h != hash {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("invalid md5: %s: error removing %s: %v", h, path, err)
		}
		return fmt.Errorf("invalid md5: %s", h)
	}
	return nil
}
