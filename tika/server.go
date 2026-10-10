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
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

// Server represents a Tika server. Create a new Server with NewServer,
// start it with Start, and shut it down with the close function returned
// from Start.
// There is no need to create a Server for an already running Tika Server
// since you can pass its URL directly to a Client.
// Additional Java system properties can be added to a Taka Server before
// startup by adding to the JavaProps map.
// Tika Server 2.x requires Java 8 or later, Tika Server 3.x requires
// Java 11 or later and Tika Server 4.x requires Java 17 or later.
type Server struct {
	jar       string
	url       string // url is derived from port.
	port      string
	cmd       *exec.Cmd
	child     *ChildOptions
	JavaProps map[string]string
	// ConfigPath is the path to a tika-config file passed to the server
	// with the -c flag. If empty, the flag is not included. The file is XML
	// in Tika Server 2.x and 3.x, and JSON in Tika Server 4.x. In Tika Server
	// 2.x and 3.x, settings such as forking, timeouts and max files are
	// configured in the <server><params> section of this file.
	ConfigPath string
}

// ChildOptions represent command line parameters that can be used when Tika is run with the -spawnChild option.
// If a field is less than or equal to 0, the associated flag is not included.
//
// Deprecated: ChildOptions are only supported by Tika Server 1.x. Tika Server
// 2.x and later fork a child process by default; configure <server><params>
// in a tika-config.xml and set Server.ConfigPath instead.
type ChildOptions struct {
	MaxFiles          int
	TaskPulseMillis   int
	TaskTimeoutMillis int
	PingPulseMillis   int
	PingTimeoutMillis int
}

func (co *ChildOptions) args() []string {
	if co == nil {
		return nil
	}
	args := []string{}
	args = append(args, "-spawnChild")
	if co.MaxFiles == -1 || co.MaxFiles > 0 {
		args = append(args, "-maxFiles", strconv.Itoa(co.MaxFiles))
	}
	if co.TaskPulseMillis > 0 {
		args = append(args, "-taskPulseMillis", strconv.Itoa(co.TaskPulseMillis))
	}
	if co.TaskTimeoutMillis > 0 {
		args = append(args, "-taskTimeoutMillis", strconv.Itoa(co.TaskTimeoutMillis))
	}
	if co.PingPulseMillis > 0 {
		args = append(args, "-pingPulseMillis", strconv.Itoa(co.PingPulseMillis))
	}
	if co.PingTimeoutMillis > 0 {
		args = append(args, "-pingTimeoutMillis", strconv.Itoa(co.PingTimeoutMillis))
	}
	return args
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

	// Check if the jar file exists.
	if _, err := os.Stat(jar); os.IsNotExist(err) {
		return nil, fmt.Errorf("jar file %q does not exist", jar)
	}

	if port == "" {
		port = "9998"
	}

	urlString := "http://localhost:" + port
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %v", port, err)
	}

	s := &Server{
		jar:       jar,
		port:      port,
		url:       u.String(),
		JavaProps: map[string]string{},
	}

	return s, nil
}

// ChildMode sets up the server to use the -spawnChild option.
// If used, ChildMode must be called before starting the server.
// If you want to turn off the -spawnChild option, call Server.ChildMode(nil).
//
// Deprecated: ChildMode is only supported by Tika Server 1.x, and Tika Server
// 2.x and later will fail to start with it. Configure <server><params> in a
// tika-config.xml and set Server.ConfigPath instead.
func (s *Server) ChildMode(ops *ChildOptions) error {
	if s.cmd != nil {
		return fmt.Errorf("server process already started, cannot switch to spawn child mode")
	}
	s.child = ops
	return nil
}

var command = exec.Command

// Start starts the given server. Start will start a new Java process. The
// caller must call Stop() to shut down the process when finished with the
// Server. Start will wait for the server to be available or until ctx is
// cancelled.
func (s *Server) Start(ctx context.Context) error {
	if _, err := os.Stat(s.jar); os.IsNotExist(err) {
		return err
	}

	// Create a slice of Java system properties to be passed to the JVM.
	props := []string{}
	for k, v := range s.JavaProps {
		props = append(props, fmt.Sprintf("-D%s=%q", k, v))
	}

	args := append(props, "-jar", s.jar, "-p", s.port)
	if s.ConfigPath != "" {
		args = append(args, "-c", s.ConfigPath)
	}
	args = append(args, s.child.args()...)
	cmd := command("java", args...)

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
// If not running in a Windows environment, it is recommended to use Shutdown
// for a more graceful shutdown of the Java process.
func (s *Server) Stop() error {
	if err := s.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("could not kill server: %v", err)
	}
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("could not wait for server to finish: %v", err)
	}
	return nil
}

// Shutdown attempts to close the server gracefully before using SIGKILL,
// Stop() uses SIGKILL right away, which causes the kernal to stop the java process instantly.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("could not interrupt server: %v", err)
	}
	errChannel := make(chan error)
	go func() {
		select {
		case errChannel <- s.cmd.Wait():
		case <-ctx.Done():
		}
	}()
	select {
	case err := <-errChannel:
		if err != nil && !interruptExit(err) {
			return fmt.Errorf("could not wait for server to finish: %v", err)
		}
	case <-ctx.Done():
		if err := s.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("could not kill server: %v", err)
		}
	}
	return nil
}

// interruptExit reports whether err is the exit status of a Java process that
// shut down because of os.Interrupt. The JVM runs its shutdown hooks and exits
// with status 130 (128 + SIGINT) when interrupted.
func interruptExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 130
}

func sha512Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// A Version represents a Tika Server version.
type Version string

// Deprecated/end-of-life versions of Tika Server.
const (
	Version119 Version = "1.19"
	Version120 Version = "1.20"
	Version121 Version = "1.21"
)

// Supported versions of Tika Server.
const (
	Version260 Version = "2.6.0"
	Version270 Version = "2.7.0"
	Version280 Version = "2.8.0"
	Version294 Version = "2.9.4"
	Version300 Version = "3.0.0"
	Version310 Version = "3.1.0"
	Version323 Version = "3.2.3"
	Version332 Version = "3.3.2"
	Version410 Version = "4.1.0"
)

// Versions is a list of supported versions of Apache Tika.
var Versions = []Version{Version260, Version270, Version280, Version294, Version300, Version310, Version323, Version332, Version410}

// sha512s holds the sha512 of the download for each version: the server JAR
// for Tika 3.x and earlier, and the server zip for Tika 4.x and later.
var sha512s = map[Version]string{
	Version119: "a9e2b6186cdb9872466d3eda791d0e1cd059da923035940d4b51bb1adc4a356670fde46995725844a2dd500a09f3a5631d0ca5fbc2d61a59e8e0bd95c9dfa6c2",
	Version120: "a7ef35317aba76be8606f9250893efece8b93384e835a18399da18a095b19a15af591e3997828d4ebd3023f21d5efad62a91918610c44e692cfd9bed01d68382",
	Version121: "e705c836b2110530c8d363d05da27f65c4f6c9051b660cefdae0e5113c365dbabed2aa1e4171c8e52dbe4cbaa085e3d8a01a5a731e344942c519b85836da646c",
	Version260: "df72b1179c39c1a70daaf19a43acc1a2c7e6ae7aeae2bf9aa4a1a447ac460324d50ba1a98da81d4a996cea0a86b68cb168ae134b4b1561dea278a245b02d591a",
	Version270: "23759bacee231bc700765d4f934da9eeabfa8e407381b72472d66da532509f2edd976bce33fd38c34a8bf43b2b28a1e19ab9ddfc187f9cd96f8a31a285248e13",
	Version280: "e8a23ce98c412a5c157e002934270a0b18ce86004dee0ffded7ddc6482b6b3931ccb7ed7f13853d16697e85e3d2785ae6893e247cf90561c845b6d645f8da61c",
	Version294: "7498f6e87cc331fa14a994594f941d8935d6bf7b11407ea2ff893b1acd8f19c38dca45f1e651a736eb7ba963ab1801833e56e02096d82d26c8650c48ffe1c2ce",
	Version300: "9228dbf437d065f23c59f33a3dd26f4d95e6339dd0a362714acae59d89a23e910838f4ed6b870e7b780d027786f4db1a07b194033f225258a95d133e2b000bba",
	Version310: "e9f6df28329cb36519b748e04eb9c96db2e776f4bfafcb92a48b799e9448ee182c908eeb3e807a98f6c59af79a01e72022d7f5654764342b6d6688c288817c8e",
	Version323: "3099b58451a74e940f8a4f76933e0de86bd4dba70efd1b645df1926988b31dac7f1196efda43281aa8d88b1e29ef73adf11f069b7a27d6baf753a83bf95f5f86",
	Version332: "fb1f2fe57ac458b09d44d41d816f582e1d2fc93488acff6275caf414d8d5ef94e42166edc0b488dc2fb6ef3aa21fab62b107c43b9060385ff6d675e393c2c9e9",
	Version410: "c932f84c569fb02df4f72ab060d6f2cf77b6797417040202401bc256e18a8f6f0ebc25d12d6dce222cb9328abfcb388cfa6511d7382c765a45687240a8a4bd7e",
}

// majorVersion returns the major version of the version string v, such as 4
// for "4.1.0", or 0 if it cannot be parsed.
func majorVersion(v string) int {
	major, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return 0
	}
	return n
}

// isZipDistribution reports whether v is distributed as a zip containing the
// server JAR and the lib/ directory it depends on, rather than a single JAR.
func isZipDistribution(v Version) bool {
	return majorVersion(string(v)) >= 4
}

// downloadURL returns the URL of the Tika Server download for v. Tika 1.x was
// published to Maven Central as tika-server, and Tika 2.x and 3.x as
// tika-server-standard. Tika 4.x and later are only published as a zip on the
// Apache archive.
func downloadURL(v Version) string {
	if isZipDistribution(v) {
		return fmt.Sprintf("https://archive.apache.org/dist/tika/%s/tika-server-standard-%s.zip", v, v)
	}
	artifact := "tika-server-standard"
	if strings.HasPrefix(string(v), "1.") {
		artifact = "tika-server"
	}
	return fmt.Sprintf("https://repo1.maven.org/maven2/org/apache/tika/%s/%s/%s-%s.jar", artifact, v, artifact, v)
}

// DownloadServer downloads and validates the given server version,
// saving it at path. DownloadServer returns an error if it could
// not be downloaded/validated.
// It is the caller's responsibility to remove the file when no longer needed.
// If the file already exists and has the correct sha512, DownloadServer will
// do nothing.
func DownloadServer(ctx context.Context, v Version, path string) error {
	if sha512s[v] == "" {
		return fmt.Errorf("unsupported Tika version: %s", v)
	}
	if isZipDistribution(v) {
		return fmt.Errorf("version %s is distributed as a zip: use DownloadServerDir", v)
	}
	return download(ctx, v, path)
}

// DownloadServerDir downloads and validates the given server version into
// dir, creating dir if needed, and returns the path of the server JAR to pass
// to NewServer. DownloadServerDir returns an error if the server could not be
// downloaded/validated.
func DownloadServerDir(ctx context.Context, v Version, dir string) (string, error) {
	if sha512s[v] == "" {
		return "", fmt.Errorf("unsupported Tika version: %s", v)
	}
	if !isZipDistribution(v) {
		return "", fmt.Errorf("version %s is distributed as a JAR: use DownloadServer", v)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("error creating directory: %v", err)
	}

	zipPath := filepath.Join(dir, path.Base(downloadURL(v)))
	if err := download(ctx, v, zipPath); err != nil {
		return "", err
	}
	jarName := fmt.Sprintf("tika-server-standard-%s.jar", v)
	jar := filepath.Join(dir, jarName)
	if _, err := os.Stat(jar); err == nil {
		return jar, nil
	}
	if err := unzip(zipPath, dir, jarName); err != nil {
		return "", fmt.Errorf("error extracting %s: %v", zipPath, err)
	}
	if _, err := os.Stat(jar); err != nil {
		return "", fmt.Errorf("server JAR %s not found in %s", jarName, zipPath)
	}
	return jar, nil
}

// unzip extracts the zip file src into dir. The jarName is extracted after
// all others, so that its presence shows that all other files are extracted.
func unzip(src, dir, jarName string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	files := make([]*zip.File, 0, len(r.File))
	var lastFile *zip.File
	for _, f := range r.File {
		if f.Name == jarName {
			lastFile = f
			continue
		}
		files = append(files, f)
	}
	if lastFile != nil {
		files = append(files, lastFile)
	}
	for _, f := range files {
		if err := unzipFile(f, dir); err != nil {
			return err
		}
	}
	return nil
}

func unzipFile(f *zip.File, dir string) error {
	name := filepath.FromSlash(f.Name)
	// Reject paths that would be written outside of dir.
	if !filepath.IsLocal(name) {
		return fmt.Errorf("invalid file path %q", f.Name)
	}
	p := filepath.Join(dir, name)
	if f.FileInfo().IsDir() {
		return os.MkdirAll(p, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	perm := f.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// download downloads the file for v to path and validates its sha512. If
// the file already exists and has the correct sha512, download does nothing.
func download(ctx context.Context, v Version, path string) error {
	hash := sha512s[v]
	if got, err := sha512Hash(path); err == nil {
		if got == hash {
			return nil
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer out.Close()

	url := downloadURL(v)
	resp, err := ctxhttp.Get(ctx, nil, url)
	if err != nil {
		return fmt.Errorf("unable to download %q: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unable to download %q: response code %d", url, resp.StatusCode)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("error saving download: %v", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("error saving download: %v", err)
	}

	h, err := sha512Hash(path)

	if err != nil {
		return err
	}
	if h != hash {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("invalid sha512: %s: error removing %s: %v", h, path, err)
		}
		return fmt.Errorf("invalid sha512: %s", h)
	}
	return nil
}
