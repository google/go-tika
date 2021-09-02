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

func TestLanguage(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.Language(context.Background(), nil)
	if err != nil {
		t.Errorf("Language returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("Language got %q, want %q", got, want)
	}
}

func TestLanguageString(t *testing.T) {
	want := "test value"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))
	defer ts.Close()
	c := NewClient(nil, ts.URL)
	got, err := c.LanguageString(context.Background(), "")
	if err != nil {
		t.Errorf("LanguageString returned an error: %v, want %q", err, want)
	}
	if got != want {
		t.Errorf("LanguageString got %q, want %q", got, want)
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
