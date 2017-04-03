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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
)

// Client represents a connection to a Tika Server.
type Client struct {
	// url is the URL of the Tika Server, including the port (if necessary), but
	// not the trailing slash. For example, http://localhost:9998.
	url string
	// HTTPClient is the client that will be used to call the Tika Server. If no
	// client is specified, a default client will be used. Since http.Clients are
	// thread safe, the same client will be used for all requests by this Client.
	httpClient *http.Client
}

// NewClient creates a new Client. If httpClient is nil, the http.DefaultClient will be
// used.
func NewClient(httpClient *http.Client, urlString string) *Client {
	return &Client{httpClient: httpClient, url: urlString}
}

// A Parser represents a Tika Parser. To get a list of all Parsers, see Parsers().
type Parser struct {
	Name           string
	Decorated      bool
	Composite      bool
	Children       []Parser
	SupportedTypes []string
}

// MimeType represents a Tika Mime Type. To get a list of all MimeTypes, see
// MimeTypes.
type MimeType struct {
	Alias     []string
	SuperType string
}

// A Detector represents a Tika Detector. Detectors are used to get the filetype
// of a file. To get a list of all Detectors, see Detectors().
type Detector struct {
	Name      string
	Composite bool
	Children  []Detector
}

// Translator represents the Java package of a Tika Translator.
type Translator string

// Translators available by defult in Tika. You must configure all required
// authentication details in Tika Server (for example, an API key).
const (
	Lingo24Translator   Translator = "org.apache.tika.language.translate.Lingo24Translator"
	GoogleTranslator    Translator = "org.apache.tika.language.translate.GoogleTranslator"
	MosesTranslator     Translator = "org.apache.tika.language.translate.MosesTranslator"
	JoshuaTranslator    Translator = "org.apache.tika.language.translate.JoshuaTranslator"
	MicrosoftTranslator Translator = "org.apache.tika.language.translate.MicrosoftTranslator"
	YandexTranslator    Translator = "org.apache.tika.language.translate.YandexTranslator"
)

// XTIKAContent is the metadata field of the content of a file after recursive
// parsing. See ParseRecursive and MetaRecursive.
const XTIKAContent = "X-TIKA:content"

// call makes the given request to c and returns the result as a []byte and
// error. call returns an error if the response code is not 2xx.
func (c *Client) call(input io.Reader, method, path string, header http.Header) ([]byte, error) {
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}

	req, err := http.NewRequest(method, c.url+path, input)
	if err != nil {
		return nil, err
	}
	req.Header = header

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("response code %v", resp.StatusCode)
	}

	return ioutil.ReadAll(resp.Body)
}

// callString makes the given request to c and returns the result as a string
// and error. callString returns an error if the response code is not 2xx.
func (c *Client) callString(input io.Reader, method, path string, header http.Header) (string, error) {
	body, err := c.call(input, method, path, header)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Parse parses the given input, returning the body of the input and an error.
// If the error is not nil, the body is undefined.
func (c *Client) Parse(input io.Reader) (string, error) {
	return c.callString(input, "PUT", "/tika", nil)
}

// ParseRecursive parses the given input and all embedded documents, returning a
// list of the contents of the input with one element per document. See
// MetaRecursive for access to all metadata fields. If the error is not nil, the
// result is undefined.
func (c *Client) ParseRecursive(input io.Reader) ([]string, error) {
	m, err := c.MetaRecursive(input)
	if err != nil {
		return nil, err
	}
	var r []string
	for _, d := range m {
		if content := d[XTIKAContent]; len(content) > 0 {
			r = append(r, content[0])
		}
	}
	return r, nil
}

// Meta parses the metadata from the given input, returning the metadata and an
// error. If the error is not nil, the metadata is undefined.
func (c *Client) Meta(input io.Reader) (string, error) {
	return c.callString(input, "PUT", "/meta", nil)
}

// MetaField parses the metadata from the given input and returns the given
// field. If the error is not nil, the result string is undefined.
func (c *Client) MetaField(input io.Reader, field string) (string, error) {
	return c.callString(input, "PUT", fmt.Sprintf("/meta/%v", field), nil)
}

// Detect gets the mimetype of the given input, returning the mimetype and an
// error. If the error is not nil, the mimetype is undefined.
func (c *Client) Detect(input io.Reader) (string, error) {
	return c.callString(input, "PUT", "/detect/stream", nil)
}

// Language detects the language of the given input, returning the two letter
// language code and an error. If the error is not nil, the language is
// undefined.
func (c *Client) Language(input io.Reader) (string, error) {
	return c.callString(input, "PUT", "/language/stream", nil)
}

// LanguageString detects the language of the given string, returning the two letter
// language code and an error. If the error is not nil, the language is
// undefined.
func (c *Client) LanguageString(input string) (string, error) {
	r := strings.NewReader(input)
	return c.callString(r, "PUT", "/language/string", nil)
}

// MetaRecursive parses the given input and all embedded documents. The result
// is a list of maps from metadata key to value for each document. The content
// of each document is in the XTIKAContent field. See ParseRecursive to just get
// the content of each document. If the error is not nil, the result list is
// undefined.
func (c *Client) MetaRecursive(input io.Reader) ([]map[string][]string, error) {
	body, err := c.call(input, "PUT", "/rmeta/text", nil)
	if err != nil {
		return nil, err
	}
	var m []map[string]interface{}
	err = json.Unmarshal(body, &m)
	if err != nil {
		return nil, err
	}
	var r []map[string][]string
	for _, d := range m {
		doc := make(map[string][]string)
		r = append(r, doc)
		for k, v := range d {
			switch v.(type) {
			case string:
				doc[k] = []string{v.(string)}
			case []interface{}:
				doc[k] = []string{}
				for _, i := range v.([]interface{}) {
					switch i.(type) {
					case string:
						doc[k] = append(doc[k], i.(string))
					default:
						return nil, fmt.Errorf("field %q has value %v and type %v, expected a string or []string", k, v, reflect.TypeOf(v))
					}
				}
			default:
				return nil, fmt.Errorf("field %q has value %v and type %v, expected a string or []string", k, v, reflect.TypeOf(v))
			}
		}
	}
	return r, nil
}

// Translate returns an error and the translated input from src language to
// dst language using t. If the error is not nil, the translation is undefined.
func (c *Client) Translate(input io.Reader, t Translator, src, dst string) (string, error) {
	return c.callString(input, "POST", fmt.Sprintf("/translate/all/%s/%s/%s", t, src, dst), nil)
}

var jsonHeader = http.Header{"Accept": []string{"application/json"}}

// Parsers returns the list of available parsers and an error. If the error is
// not nil, the list is undefined. To get all available parsers, iterate through
// the Children of every Parser.
func (c *Client) Parsers() (*Parser, error) {
	body, err := c.call(nil, "GET", "/parsers/details", jsonHeader)
	if err != nil {
		return nil, err
	}
	var parsers Parser
	err = json.Unmarshal(body, &parsers)
	return &parsers, err
}

// Version returns the default hello message from Tika server.
func (c *Client) Version() (string, error) {
	return c.callString(nil, "GET", "/version", nil)
}

// MimeTypes returns a map from Mime Type name to MimeType, or properties about
// that specific Mime Type.
func (c *Client) MimeTypes() (map[string]MimeType, error) {
	body, err := c.call(nil, "GET", "/mime-types", jsonHeader)
	if err != nil {
		return nil, err
	}
	var mt map[string]MimeType
	err = json.Unmarshal(body, &mt)
	return mt, err
}

// Detectors returns the list of available Detectors for this server. To get all
// available detectors, iterate through the Children of every Detector.
func (c *Client) Detectors() (*Detector, error) {
	body, err := c.call(nil, "GET", "/detectors", jsonHeader)
	if err != nil {
		return nil, err
	}
	var d Detector
	err = json.Unmarshal(body, &d)
	return &d, err
}
