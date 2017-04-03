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
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// Server represents a running Tika server. Start a new Server with StartServer and
// shut it down with Close. There is no need to create a Server for an already
// running Tika Server - you can pass its URL directly to a Client.
type Server struct {
	url     string
	cmd     *exec.Cmd
	done    chan error
	timeout time.Duration
}

// URL returns the URL of this Server.
func (s *Server) URL() string {
	return s.url
}

// A ServerConfig can be used to configure a Server before starting it.
type ServerConfig struct {
	Port     string        // Port is the port the Server will run on (default "9998").
	Timeout  time.Duration // How long to wait for the server to start (default 10s).
	Hostname string        // The host name to use (default localhost).
}

type commander func(string, ...string) *exec.Cmd

// cmder is used to stub out *exec.Cmd for testing.
var cmder commander = exec.Command

// StartServer creates a new Server. StartServer will start a new Java process. The
// caller must call Close() when finished with the Server.
func StartServer(jar string, config *ServerConfig) (*Server, error) {
	if jar == "" {
		return nil, fmt.Errorf("no jar file specified")
	}
	if config == nil {
		config = &ServerConfig{}
	}
	if config.Port == "" {
		config.Port = "9998"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Hostname == "" {
		config.Hostname = "localhost"
	}
	if _, err := os.Stat(jar); err != nil {
		return nil, fmt.Errorf("jar file not found: %s", jar)
	}
	urlString := "http://" + config.Hostname + ":" + config.Port
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %v", urlString, err)
	}
	urlString = u.String()

	c := cmder("java", "-jar", jar, "-p", config.Port)
	done := make(chan error, 1)

	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, err
	}
	s := &Server{
		cmd:     c,
		done:    done,
		url:     urlString,
		timeout: config.Timeout,
	}

	if err := c.Start(); err != nil {
		return nil, err
	}
	go func() {
		done <- c.Wait()
	}()

	if err := s.waitForStart(); err != nil {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(stderr); err != nil {
			return nil, fmt.Errorf("error reading stderr: %v", err)
		}
		return nil, fmt.Errorf("%v: %v", err, buf.String())
	}
	return s, nil
}

// waitForServer waits until the given Server is responding to requests.
// waitForStart returns an error if the server does not respond within 10 seconds.
func (s Server) waitForStart() error {
	c := NewClient(nil, s.url)
	var err error
	for i := time.Duration(0); i < s.timeout; i += time.Second {
		if _, err = c.Version(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

// Close shuts the Server down, releasing resources. Callers are responsible
// for calling Close after calling StartServer.
func (s *Server) Close() error {
	if s.cmd == nil {
		return errors.New("Close called on invalid Server: did you call StartServer?")
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

// DownloadServer downloads and validates the given server version,
// saving it at path. DownloadServer returns an error if it could
// not be downloaded/validated. Valid values for the version are 1.14.
func DownloadServer(version, path string) error {
	md5s := map[string]string{
		"1.14": "39055fc71358d774b9da066f80b1141c",
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

	url := "http://search.maven.org/remotecontent?filepath=org/apache/tika/tika-server/" + version + "/tika-server-" + version + ".jar"
	resp, err := http.Get(url)
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
