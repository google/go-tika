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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

// errorServer always responds with http.StatusInternalServerError.
var errorServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}))

var errorClient = NewClient(nil, errorServer.URL)

func TestMain(m *testing.M) {
	r := m.Run()
	errorServer.Close()
	os.Exit(r)
}

func TestCallError(t *testing.T) {
	tests := []struct {
		method string
		url    string
	}{
		{"bad method", ""},
		{"GET", "https://unknown_test_url"},
	}
	for _, test := range tests {
		c := NewClient(nil, test.url)
		if _, err := c.call(context.Background(), nil, test.method, "", nil); err == nil {
			t.Errorf("call(%q, %q) got no error, want error", test.method, test.url)
		}

	}
}

func TestParse(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Parse(context.Background(), nil)
	if err != nil {
		t.Fatalf("Parse returned nil, want %q", want)
	}
	if got != want {
		t.Errorf("Parse got %q, want %q", got, want)
	}
}

func TestParseWithHeader(t *testing.T) {
	want := "test value"
	wantHeader := "application/json"
	gotHeader := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Accept")
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	hdr := http.Header{}
	hdr["Accept"] = []string{"application/json"}
	c := NewClient(nil, ts.URL)
	got, err := c.ParseWithHeader(context.Background(), nil, hdr)
	if err != nil {
		t.Fatalf("Parse returned nil, want %q", want)
	}
	if got != want {
		t.Errorf("Parse got %q, want %q", got, want)
	}
	if gotHeader != wantHeader {
		t.Errorf("ParseWithHeader Header incorrect got %q, want %q", gotHeader, wantHeader)
	}
}

func TestParseReader(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	body, err := c.ParseReader(context.Background(), nil)
	if err != nil {
		t.Fatalf("ParseReader returned nil, want %q", want)
	}
	defer body.Close()
	got, err := ioutil.ReadAll(body)
	if err != nil {
		t.Fatalf("Reading the returned body failed: %v", err)
	}
	if s := string(got); s != want {
		t.Errorf("ParseReader got %q, want %q", s, want)
	}
}

func TestParseRecursive(t *testing.T) {
	tests := []struct {
		response   string
		want       []string
		statusCode int
	}{
		{
			response: `[{"X-TIKA:content":"test 1"}]`,
			want:     []string{"test 1"},
		},
		{
			response: `[{"X-TIKA:content":"test 1"},{"X-TIKA:content":"test 2"}]`,
			want:     []string{"test 1", "test 2"},
		},
		{
			response: `[{"other_key":"other_value"},{"X-TIKA:content":"test"}]`,
			want:     []string{"test"},
		},
		{
			response: `[{"tk:content":"test 1"}]`,
			want:     []string{"test 1"},
		},
		{
			response: `[{"tk:content":"test 1"},{"tk:content":"test 2"}]`,
			want:     []string{"test 1", "test 2"},
		},
		{
			response: `[{"tk:content":"test tk","X-TIKA:content":"test xtika"}]`,
			want:     []string{"test tk"},
		},
		{
			response: `[]`,
		},
		{
			statusCode: http.StatusUnprocessableEntity,
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if test.statusCode != 0 {
				w.WriteHeader(test.statusCode)
			} else {
				fmt.Fprint(w, test.response)
			}
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.ParseRecursive(context.Background(), nil)
		if err != nil {
			if test.statusCode != 0 {
				var tikaErr ClientError
				if errors.As(err, &tikaErr) {
					if tikaErr.StatusCode != test.statusCode {
						t.Errorf("ParseRecursive expected status code %d, got %d", test.statusCode, tikaErr.StatusCode)
					}
				} else {
					t.Errorf("ParseRecursive expected TikaError, got %T", err)
				}
			} else {
				t.Errorf("ParseRecursive returned an error: %v, want %v", err, test.want)
			}
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("ParseRecursive(%q) got %v, want %v", test.response, got, test.want)
		}
	}
}

func TestParseRecursiveError(t *testing.T) {
	if _, err := errorClient.ParseRecursive(context.Background(), nil); err == nil {
		t.Error("ParseRecursive got no error, want an error")
	}
}

func TestTKContentConstant(t *testing.T) {
	if TKContent != "tk:content" {
		t.Errorf("TKContent = %q, want %q", TKContent, "tk:content")
	}
}

func TestMeta(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Meta(context.Background(), nil)
	if err != nil {
		t.Fatalf("Meta returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("Meta got %q, want %q", got, want)
	}
}

func TestMetaWithHeader(t *testing.T) {
	want := "test value"
	wantHeader := "application/json"
	gotHeader := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Accept")
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	hdr := http.Header{}
	hdr["Accept"] = []string{"application/json"}
	got, err := c.MetaWithHeader(context.Background(), nil, hdr)
	if err != nil {
		t.Fatalf("MetaWithHeader returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("MetaWithHeader got %q, want %q", got, want)
	}
	if gotHeader != wantHeader {
		t.Errorf("TestMetaWithHeader Header incorrect got %q, want %q", gotHeader, wantHeader)
	}
}

func TestMetaField(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.MetaField(context.Background(), nil, "")
	if err != nil {
		t.Errorf("MetaField returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("MetaField got %q, want %q", got, want)
	}

}

func TestMetaFieldWithHeader(t *testing.T) {
	want := "test value"
	wantHeader := "application/json"
	gotHeader := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Accept")
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	hdr := http.Header{}
	hdr["Accept"] = []string{"application/json"}
	got, err := c.MetaFieldWithHeader(context.Background(), nil, "", hdr)
	if err != nil {
		t.Errorf("MetaFieldWithHeader returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("MetaFieldWithHeader got %q, want %q", got, want)
	}
	if gotHeader != wantHeader {
		t.Errorf("TestMetaFieldWithHeader Header incorrect got %q, want %q", gotHeader, wantHeader)
	}
}

func TestDetect(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Detect(context.Background(), nil)
	if err != nil {
		t.Errorf("Detect returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("Detect got %q, want %q", got, want)
	}
}

func TestDetectServerVersions(t *testing.T) {
	tests := []struct {
		name          string
		versionStatus int
		version       string
		wantPath      string
	}{
		{"Tika 2.x", http.StatusOK, "Apache Tika 2.9.4", "/detect/stream"},
		{"Tika 3.x", http.StatusOK, "Apache Tika 3.3.2", "/detect/stream"},
		{"Tika 4.x", http.StatusOK, "Apache Tika 4.1.0\n", "/detect"},
		{"unknown version", http.StatusOK, "unknown", "/detect/stream"},
		{"version error", http.StatusNotFound, "", "/detect/stream"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := "text/html"
			versionCalls := 0
			var gotPaths []string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/version" {
					versionCalls++
					w.WriteHeader(test.versionStatus)
					fmt.Fprint(w, test.version)
					return
				}
				gotPaths = append(gotPaths, r.URL.Path)
				if r.URL.Path != test.wantPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fmt.Fprint(w, want)
			}))
			defer ts.Close()

			c := NewClient(nil, ts.URL)
			// Call Detect twice to check the server version is only requested once.
			for i := 0; i < 2; i++ {
				got, err := c.Detect(context.Background(), strings.NewReader("<html></html>"))
				if err != nil {
					t.Fatalf("Detect returned an error: %v, want %q", err, want)
				}
				if got != want {
					t.Errorf("Detect got %q, want %q", got, want)
				}
			}
			if versionCalls != 1 {
				t.Errorf("Detect called /version %d times, want 1", versionCalls)
			}
			if wantPaths := []string{test.wantPath, test.wantPath}; !reflect.DeepEqual(gotPaths, wantPaths) {
				t.Errorf("Detect called %v, want %v", gotPaths, wantPaths)
			}
		})
	}
}

func TestDetectConnectionError(t *testing.T) {
	c := NewClient(nil, "https://unknown_test_url")
	if _, err := c.Detect(context.Background(), nil); err == nil {
		t.Error("Detect got no error, want an error")
	}
}

func TestLanguage(t *testing.T) {
	// Tika 4.x: /language returns 200, /language/stream is not used.
	{
		want := "en"
		inputBody := "hello world"
		streamCalled := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/language/stream" {
				streamCalled = true
			}
			if r.URL.Path == "/language" {
				body, _ := ioutil.ReadAll(r.Body)
				if string(body) != inputBody {
					t.Errorf("/language got body %q, want %q", string(body), inputBody)
				}
				fmt.Fprint(w, want)
				return
			}
			http.NotFound(w, r)
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.Language(context.Background(), strings.NewReader(inputBody))
		if err != nil {
			t.Errorf("Language returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("Language got %q, want %q", got, want)
		}
		if streamCalled {
			t.Errorf("Language called /language/stream unexpectedly")
		}
	}

	// Legacy fallback: /language returns 404, fallback to /language/stream succeeds with preserved body.
	{
		want := "fr"
		inputBody := "bonjour le monde"
		var gotFallbackBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/language":
				w.WriteHeader(http.StatusNotFound)
			case "/language/stream":
				body, _ := ioutil.ReadAll(r.Body)
				gotFallbackBody = string(body)
				fmt.Fprint(w, want)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.Language(context.Background(), strings.NewReader(inputBody))
		if err != nil {
			t.Errorf("Language fallback returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("Language fallback got %q, want %q", got, want)
		}
		if gotFallbackBody != inputBody {
			t.Errorf("Language fallback got body %q, want %q", gotFallbackBody, inputBody)
		}
	}

	// Legacy fallback: /language returns 405 (Method Not Allowed), fallback succeeds.
	{
		want := "de"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/language":
				w.WriteHeader(http.StatusMethodNotAllowed)
			case "/language/stream":
				fmt.Fprint(w, want)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.Language(context.Background(), strings.NewReader("guten tag"))
		if err != nil {
			t.Errorf("Language fallback on 405 returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("Language fallback on 405 got %q, want %q", got, want)
		}
	}

	// Server error: 500 from /language does NOT trigger fallback.
	{
		streamCalled := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/language/stream" {
				streamCalled = true
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		if _, err := c.Language(context.Background(), strings.NewReader("hello")); err == nil {
			t.Errorf("Language on 500 got nil error, want error")
		}
		if streamCalled {
			t.Errorf("Language triggered fallback on 500")
		}
	}

	// Nil input: works without error.
	{
		want := "es"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/language" {
				fmt.Fprint(w, want)
			}
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.Language(context.Background(), nil)
		if err != nil {
			t.Errorf("Language(nil) returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("Language(nil) got %q, want %q", got, want)
		}
	}

	// Buffering error: if reader fails, return error without making a request.
	{
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected HTTP request made when reader failed")
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		errReader := &testErrReader{err: errors.New("read error")}
		if _, err := c.Language(context.Background(), errReader); err == nil {
			t.Errorf("Language with failing reader got nil error, want error")
		}
	}
}

type testErrReader struct {
	err error
}

func (r *testErrReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func TestLanguageString(t *testing.T) {
	// Tika 4.x: /language returns 200, /language/string is not used.
	{
		want := "en"
		inputBody := "hello world"
		stringCalled := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/language/string" {
				stringCalled = true
			}
			if r.URL.Path == "/language" {
				body, _ := ioutil.ReadAll(r.Body)
				if string(body) != inputBody {
					t.Errorf("/language got body %q, want %q", string(body), inputBody)
				}
				fmt.Fprint(w, want)
				return
			}
			http.NotFound(w, r)
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.LanguageString(context.Background(), inputBody)
		if err != nil {
			t.Errorf("LanguageString returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("LanguageString got %q, want %q", got, want)
		}
		if stringCalled {
			t.Errorf("LanguageString called /language/string unexpectedly")
		}
	}

	// Legacy fallback: /language returns 404, fallback to /language/string succeeds with preserved body.
	{
		want := "fr"
		inputBody := "bonjour le monde"
		var gotFallbackBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/language":
				w.WriteHeader(http.StatusNotFound)
			case "/language/string":
				body, _ := ioutil.ReadAll(r.Body)
				gotFallbackBody = string(body)
				fmt.Fprint(w, want)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.LanguageString(context.Background(), inputBody)
		if err != nil {
			t.Errorf("LanguageString fallback returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("LanguageString fallback got %q, want %q", got, want)
		}
		if gotFallbackBody != inputBody {
			t.Errorf("LanguageString fallback got body %q, want %q", gotFallbackBody, inputBody)
		}
	}

	// Legacy fallback: /language returns 405 (Method Not Allowed), fallback succeeds.
	{
		want := "de"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/language":
				w.WriteHeader(http.StatusMethodNotAllowed)
			case "/language/string":
				fmt.Fprint(w, want)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		got, err := c.LanguageString(context.Background(), "guten tag")
		if err != nil {
			t.Errorf("LanguageString fallback on 405 returned an error: %v, want %q", err, want)
		}
		if got != want {
			t.Errorf("LanguageString fallback on 405 got %q, want %q", got, want)
		}
	}

	// Server error: 500 from /language does NOT trigger fallback.
	{
		stringCalled := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/language/string" {
				stringCalled = true
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		c := NewClient(nil, ts.URL)
		if _, err := c.LanguageString(context.Background(), "hello"); err == nil {
			t.Errorf("LanguageString on 500 got nil error, want error")
		}
		if stringCalled {
			t.Errorf("LanguageString triggered fallback on 500")
		}
	}
}

func TestMetaRecursive(t *testing.T) {
	tests := []struct {
		response string
		want     []map[string][]string
	}{
		{
			response: `[{"X-TIKA:content":"test 1"}]`,
			want: []map[string][]string{
				{"X-TIKA:content": {"test 1"}},
			},
		},
		{
			response: `[{"X-TIKA:content":"test 1"},{"X-TIKA:content":"test 2"}]`,
			want: []map[string][]string{
				{"X-TIKA:content": {"test 1"}},
				{"X-TIKA:content": {"test 2"}},
			},
		},
		{
			response: `[{"other_key":"other_value"},{"X-TIKA:content":"test"}]`,
			want: []map[string][]string{
				{"other_key": {"other_value"}},
				{"X-TIKA:content": {"test"}},
			},
		},
		{
			response: `[{"other_key":["other_value", "other_value2"]}]`,
			want: []map[string][]string{
				{"other_key": {"other_value", "other_value2"}},
			},
		},
		{
			response: `[]`,
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.MetaRecursive(context.Background(), nil)
		if err != nil {
			t.Errorf("MetaRecursive returned an error: %v, want %v", err, test.want)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("MetaRecursive(%q) got %+v, want %+v", test.response, got, test.want)
		}
	}
}
func TestMetaRecursiveType(t *testing.T) {
	const (
		// Distilled forms of the X-TIKA:content in actual Tika responses.
		xml    = `<meta name="k" content="v" /> example text`
		text   = "example text"
		html   = `<meta name="k" content="v"> example text`
		ignore = ""
	)
	responsify := func(s string) []map[string][]string {
		return []map[string][]string{
			{"X-TIKA:content": {s}},
		}
	}
	tikaify := func(s string) string {
		data, err := json.Marshal(responsify(s))
		if err != nil {
			t.Errorf("error building response: %v", err)
		}
		return string(data)
	}
	tests := []struct {
		typeParam string
		want      []map[string][]string
	}{
		{
			typeParam: "",
			want:      responsify(xml),
		},
		{
			typeParam: "text",
			want:      responsify(text),
		},
		{
			typeParam: "html",
			want:      responsify(html),
		},
		{
			typeParam: "ignore",
			want:      responsify(ignore),
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mirrors the possibilities specified here:
			// https://wiki.apache.org/tika/TikaJAXRS#Recursive_Metadata_and_Content
			switch r.URL.Path {
			case "/rmeta":
				fmt.Fprint(w, tikaify(xml))
			case "/rmeta/text":
				fmt.Fprint(w, tikaify(text))
			case "/rmeta/html":
				fmt.Fprint(w, tikaify(html))
			case "/rmeta/ignore":
				fmt.Fprint(w, tikaify(ignore))
			default:
				panic("unrecognized path")
			}
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.MetaRecursiveType(context.Background(), nil, test.typeParam)
		if err != nil {
			t.Errorf("MetaRecursive returned an error: %v, want %v", err, test.want)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("MetaRecursiveType(%q) got %+v, want %+v", test.typeParam, got, test.want)
		}
	}
}
func TestMetaRecursiveError(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "invalid type",
			response: `[{"other_key":{"test": "fail"}}]`,
		},
		{
			name:     "invalid nested type",
			response: `[{"other_key":["other_value", {"test": "fail"}]}]`,
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		_, err := c.MetaRecursive(context.Background(), nil)
		if err == nil {
			t.Errorf("MetaRecursive(%s) got no error, want an error", test.name)
		}
	}
}

func TestTranslate(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Translate(context.Background(), nil, "translator", "src", "dst")
	if err != nil {
		t.Errorf("Translate returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("Translate got %q, want %q", got, want)
	}
}

func TestTranslateReader(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	body, err := c.TranslateReader(context.Background(), nil, "translator", "src", "dst")
	if err != nil {
		t.Fatalf("TranslateReader returned nil, want %q", want)
	}
	defer body.Close()
	got, err := ioutil.ReadAll(body)
	if err != nil {
		t.Fatalf("Reading the returned body failed: %v", err)
	}
	if s := string(got); s != want {
		t.Errorf("TranslateReader got %q, want %q", s, want)
	}
}

func TestParsers(t *testing.T) {
	tests := []struct {
		response string
		want     Parser
	}{
		{
			response: `{"name":"TestParser"}`,
			want: Parser{
				Name: "TestParser",
			},
		},
		{
			response: `{
				"name":"TestParser",
				"children":[
					{"name":"TestSubParser1"},
					{"name":"TestSubParser2"}
				]
			}`,
			want: Parser{
				Name: "TestParser",
				Children: []Parser{
					{
						Name: "TestSubParser1",
					},
					{
						Name: "TestSubParser2",
					},
				},
			},
		},
		{
			response: `{
				"name":"TestParser",
				"supportedTypes":["test-type"],
				"children":[
					{
						"supportedTypes":["test-type-two"],
						"name":"TestSubParser",
						"decorated":true,
						"composite":false
					}
				],
				"decorated":false,
				"composite":true}`,
			want: Parser{
				Name:           "TestParser",
				Composite:      true,
				SupportedTypes: []string{"test-type"},
				Children: []Parser{
					{
						Name:           "TestSubParser",
						Decorated:      true,
						SupportedTypes: []string{"test-type-two"},
					},
				},
			},
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.Parsers(context.Background())
		if err != nil {
			t.Errorf("Parsers returned an error: %v, want %+v", err, test.want)
		}
		if !reflect.DeepEqual(*got, test.want) {
			t.Errorf("Parsers got %+v, want %+v", got, test.want)
		}
	}
}

func TestParsersError(t *testing.T) {
	tests := []struct {
		response string
	}{
		{
			response: "invalid",
		},
		{},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		if _, err := c.Parsers(context.Background()); err == nil {
			t.Errorf("Parsers(%q) got no error, want an error", test.response)
		}
	}
	if _, err := errorClient.Parsers(context.Background()); err == nil {
		t.Errorf("Parsers got no error, want an error")
	}
}

func TestVersion(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Version(context.Background())
	if err != nil {
		t.Errorf("Version returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("Version got %q, want %q", got, want)
	}
}

func TestMIMETypes(t *testing.T) {
	tests := []struct {
		response string
		want     map[string]MIMEType
	}{
		{
			response: `{"empty-mime":{}}`,
			want: map[string]MIMEType{
				"empty-mime": {},
			},
		},
		{
			response: `{"alias-mime":{"alias":["alias1", "alias2"]}}`,
			want: map[string]MIMEType{
				"alias-mime": {
					Alias: []string{"alias1", "alias2"},
				},
			},
		},
		{
			response: `{"empty-mime":{},"super-mime":{"supertype":"super-mime"}}`,
			want: map[string]MIMEType{
				"empty-mime": {},
				"super-mime": {SuperType: "super-mime"},
			},
		},
		{
			response: `{"super-alias":{"alias":["alias1", "alias2"], "supertype": "super-mime"}}`,
			want: map[string]MIMEType{
				"super-alias": {
					Alias:     []string{"alias1", "alias2"},
					SuperType: "super-mime",
				},
			},
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.MIMETypes(context.Background())
		if err != nil {
			t.Errorf("MIMETypes returned an error: %v, want %q", err, test.want)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("MIMETypes got %+v, want %+v", got, test.want)
		}
	}
}

func TestMIMETypesError(t *testing.T) {
	tests := []struct {
		response string
	}{
		{response: ""},
		{response: `["test"]`},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		if _, err := c.MIMETypes(context.Background()); err == nil {
			t.Errorf("MIMETypes got no error, want an error")
		}
	}
	if _, err := errorClient.MIMETypes(context.Background()); err == nil {
		t.Errorf("MIMETypes got no error, want an error")
	}
}

func TestMetaRecursive_BadResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "invalid")
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.MetaRecursive(context.Background(), nil)
	if err == nil {
		t.Errorf("MetaRecursive got %q, want an error", got)
	}
}

func TestMetaRecursive_BadFieldType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"super-alias":{}`)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.MetaRecursive(context.Background(), nil)
	if err == nil {
		t.Errorf("MetaRecursive got %q, want an error", got)
	}
}

func TestDetectors(t *testing.T) {
	tests := []struct {
		response string
		want     Detector
	}{
		{
			response: `{"name":"TestDetector"}`,
			want: Detector{
				Name: "TestDetector",
			},
		},
		{
			response: `{
				"name":"TestDetector",
				"children":[
					{"name":"TestSubDetector1"},
					{"name":"TestSubDetector2"}
				]
			}`,
			want: Detector{
				Name: "TestDetector",
				Children: []Detector{
					{
						Name: "TestSubDetector1",
					},
					{
						Name: "TestSubDetector2",
					},
				},
			},
		},
		{
			response: `{
				"name":"TestDetector",
				"children":[
					{
						"name":"TestSubDetector",
						"composite":false
					}
				],
				"composite":true}`,
			want: Detector{
				Name:      "TestDetector",
				Composite: true,
				Children: []Detector{
					{
						Name: "TestSubDetector",
					},
				},
			},
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		got, err := c.Detectors(context.Background())
		if err != nil {
			t.Errorf("Detectors returned an error: %v, want %+v", err, test.want)
		}
		if !reflect.DeepEqual(*got, test.want) {
			t.Errorf("Detectors got %+v, want %+v", got, test.want)
		}
	}
}

func TestDetectorsError(t *testing.T) {
	tests := []struct {
		response string
	}{
		{
			response: "",
		},
		{
			response: `["test"]`,
		},
	}
	for _, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, test.response)
		}))
		defer ts.Close()
		c := NewClient(nil, ts.URL)
		if _, err := c.Detectors(context.Background()); err == nil {
			t.Errorf("Detectors got no error, want an error")
		}
	}
	if _, err := errorClient.Detectors(context.Background()); err == nil {
		t.Errorf("Detectors got no error, want an error")
	}
}
