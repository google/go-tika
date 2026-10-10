//go:build integration

package tika

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const integrationHTML = `<html><head><title>go-tika integration</title></head>
<body><p>The quick brown fox jumps over the lazy dog. This sentence is written in English so that language detection has enough text to work with.</p></body></html>`

// TestIntegration downloads, starts and exercises a real Tika Server. It
// requires Java and is skipped unless TIKA_VERSION is set. Set TIKA_JAR_DIR to an
// existing directory to reuse downloads between runs. Tika 4.x and later are
// extracted to a subdirectory for each version.
//
//	TIKA_VERSION=3.3.2 go test -tags integration -run Integration ./tika/
func TestIntegration(t *testing.T) {
	v := Version(os.Getenv("TIKA_VERSION"))
	if v == "" {
		t.Skip("TIKA_VERSION not set")
	}
	// The unit tests replace command with a fake server.
	oldCommand := command
	command = exec.Command
	defer func() { command = oldCommand }()

	dir := os.Getenv("TIKA_JAR_DIR")
	if dir == "" {
		dir = t.TempDir()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	jar := filepath.Join(dir, "tika-server-"+string(v)+".jar")
	if isZipDistribution(v) {
		var err error
		if jar, err = DownloadServerDir(ctx, v, filepath.Join(dir, string(v))); err != nil {
			t.Fatalf("DownloadServerDir(%q) got error: %v", v, err)
		}
	} else if err := DownloadServer(ctx, v, jar); err != nil {
		t.Fatalf("DownloadServer(%q) got error: %v", v, err)
	}

	s, err := NewServer(jar, "")
	if err != nil {
		t.Fatalf("NewServer got error: %v", err)
	}
	startCtx, startCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer startCancel()
	if err := s.Start(startCtx); err != nil {
		t.Fatalf("Start got error: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown got error: %v", err)
		}
	}()

	c := NewClient(nil, s.URL())
	input := func() *strings.Reader { return strings.NewReader(integrationHTML) }

	t.Run("Version", func(t *testing.T) {
		got, err := c.Version(ctx)
		if err != nil {
			t.Fatalf("Version got error: %v", err)
		}
		if !strings.Contains(got, string(v)) {
			t.Errorf("Version got %q, want it to contain %q", got, v)
		}
	})

	t.Run("Parse", func(t *testing.T) {
		got, err := c.Parse(ctx, input())
		if err != nil {
			t.Fatalf("Parse got error: %v", err)
		}
		if !strings.Contains(got, "quick brown fox") {
			t.Errorf("Parse got %q, want it to contain %q", got, "quick brown fox")
		}
	})

	t.Run("Meta", func(t *testing.T) {
		got, err := c.Meta(ctx, input())
		if err != nil {
			t.Fatalf("Meta got error: %v", err)
		}
		if !strings.Contains(got, "text/html") {
			t.Errorf("Meta got %q, want it to contain %q", got, "text/html")
		}
	})

	t.Run("MetaField", func(t *testing.T) {
		got, err := c.MetaField(ctx, input(), "Content-Type")
		if err != nil {
			t.Fatalf("MetaField got error: %v", err)
		}
		if !strings.Contains(got, "text/html") {
			t.Errorf("MetaField got %q, want it to contain %q", got, "text/html")
		}
	})

	t.Run("Detect", func(t *testing.T) {
		got, err := c.Detect(ctx, input())
		if err != nil {
			t.Fatalf("Detect got error: %v", err)
		}
		if got != "text/html" {
			t.Errorf("Detect got %q, want %q", got, "text/html")
		}
	})

	t.Run("Language", func(t *testing.T) {
		got, err := c.Language(ctx, input())
		if err != nil {
			t.Fatalf("Language got error: %v", err)
		}
		if strings.TrimSpace(got) != "en" {
			t.Errorf("Language got %q, want %q", got, "en")
		}
	})

	t.Run("LanguageString", func(t *testing.T) {
		got, err := c.LanguageString(ctx, "The quick brown fox jumps over the lazy dog. This sentence is written in English.")
		if err != nil {
			t.Fatalf("LanguageString got error: %v", err)
		}
		if strings.TrimSpace(got) != "en" {
			t.Errorf("LanguageString got %q, want %q", got, "en")
		}
	})

	t.Run("MetaRecursive", func(t *testing.T) {
		got, err := c.MetaRecursive(ctx, input())
		if err != nil {
			t.Fatalf("MetaRecursive got error: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("MetaRecursive got no documents")
		}
		if ct := got[0]["Content-Type"]; len(ct) == 0 || !strings.Contains(ct[0], "text/html") {
			t.Errorf("MetaRecursive got Content-Type %q, want it to contain %q", ct, "text/html")
		}
	})

	t.Run("ParseRecursive", func(t *testing.T) {
		got, err := c.ParseRecursive(ctx, input())
		if err != nil {
			t.Fatalf("ParseRecursive got error: %v", err)
		}
		if len(got) == 0 || !strings.Contains(got[0], "quick brown fox") {
			t.Errorf("ParseRecursive got %q, want the first document to contain %q", got, "quick brown fox")
		}
	})

	t.Run("Parsers", func(t *testing.T) {
		got, err := c.Parsers(ctx)
		if err != nil {
			t.Fatalf("Parsers got error: %v", err)
		}
		if got.Name == "" || len(got.Children) == 0 {
			t.Errorf("Parsers got %+v, want a named parser with children", got)
		}
	})

	t.Run("MIMETypes", func(t *testing.T) {
		got, err := c.MIMETypes(ctx)
		if err != nil {
			t.Fatalf("MIMETypes got error: %v", err)
		}
		if _, ok := got["text/html"]; !ok {
			t.Errorf("MIMETypes got %d types, want text/html to be included", len(got))
		}
	})

	t.Run("Detectors", func(t *testing.T) {
		got, err := c.Detectors(ctx)
		if err != nil {
			t.Fatalf("Detectors got error: %v", err)
		}
		if got.Name == "" {
			t.Errorf("Detectors got %+v, want a named detector", got)
		}
	})
}
