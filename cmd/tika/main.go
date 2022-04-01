/*
Copyright 2017 Google Inc. All rights reserved.
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

// The tika command provides a command line interface for Tika Server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/google/go-tika/tika"
)

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] ACTION\n\n", os.Args[0])
	fmt.Printf("ACTIONS: parse, detect, language, meta, version, parsers, mimetypes, detectors\n\n")
	fmt.Println("OPTIONS:")
	flag.PrintDefaults()
}

// Flags requiring input.
const (
	parse    = "parse"
	detect   = "detect"
	language = "language"
	meta     = "meta"
)

// Informational flags which don't require input.
const (
	version   = "version"
	parsers   = "parsers"
	mimeTypes = "mimetypes"
	detectors = "detectors"
)

// Command line flags.
var (
	downloadVersion = flag.String("download_version", "", fmt.Sprintf("Tika Server JAR version to download. If -serverJAR is specified, it will be downloaded to that location, otherwise it will be downloaded to your working directory. If the JAR has already been downloaded and has the correct MD5, this will do nothing. Valid versions: %v.", tika.Versions))
	filename        = flag.String("filename", "", "Path to file to parse.")
	metaField       = flag.String("field", "", `Specific field to get when using the "meta" action. Undefined when using the -recursive flag.`)
	recursive       = flag.Bool("recursive", false, `Whether to run "parse" or "meta" recursively, returning a list with one element per embedded document. Undefined when using the -field flag.`)
	serverJAR       = flag.String("server_jar", "", "Absolute path to the Tika Server JAR. This will start a new server, ignoring -serverURL.")
	serverURL       = flag.String("server_url", "", "URL of Tika server.")
)

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	action := flag.Arg(0)

	if *downloadVersion != "" {
		v := tika.Versions[len(tika.Versions)-1]
		supported := false
		for _, sv := range tika.Versions {
			if tika.Version(*downloadVersion) == sv {
				v = tika.Version(*downloadVersion)
				supported = true
				break
			}
		}
		if !supported {
			log.Fatalf("unsupported server version: %q", *downloadVersion)
		}
		if *serverJAR == "" {
			*serverJAR = "tika-server-" + string(v) + ".jar"
		}
		if err := tika.DownloadServer(context.Background(), v, *serverJAR); err != nil {
			log.Fatal(err)
		}
	}
	if *serverURL == "" && *serverJAR == "" {
		log.Fatal("no URL specified: set serverURL, serverJAR and/or downloadVersion")
	}

	var cancel func()
	if *serverJAR != "" {
		s, err := tika.NewServer(*serverJAR, "")
		if err != nil {
			log.Fatal(err)
		}

		err = s.Start(context.Background())
		if err != nil {
			log.Fatalf("could not start server: %v", err)
		}
		defer s.Stop()

		*serverURL = s.URL()
	}

	var file io.Reader

	// Check actions requiring input have an input and get it.
	switch action {
	case parse, detect, language, meta:
		if *filename == "" {
			cancel()
			log.Fatalf("error: you must provide an input filename")
		}
		var err error
		file, err = os.Open(*filename)
		if err != nil {
			cancel()
			log.Fatalf("error opening file: %v", err)
		}
	}

	c := tika.NewClient(nil, *serverURL)
	b, err := process(c, action, file)
	if err != nil {
		cancel()
		log.Fatalf("tika error: %v", err)
	}
	fmt.Println(b)
}

func process(c *tika.Client, action string, file io.Reader) (string, error) {
	switch action {
	default:
		flag.Usage()
		return "", fmt.Errorf("error: invalid action %q", action)
	case parse:
		if *recursive {
			bs, err := c.ParseRecursive(context.Background(), file)
			if err != nil {
				return "", err
			}
			return strings.Join(bs, "\n"), nil
		}
		return c.Parse(context.Background(), file, nil)
	case detect:
		return c.Detect(context.Background(), file)
	case language:
		return c.Language(context.Background(), file)
	case meta:
		if *metaField != "" {
			return c.MetaField(context.Background(), file, *metaField, nil)
		}
		if *recursive {
			mr, err := c.MetaRecursive(context.Background(), file)
			if err != nil {
				return "", err
			}
			bytes, err := json.MarshalIndent(mr, "", "  ")
			if err != nil {
				return "", err
			}
			return string(bytes), nil
		}
		return c.Meta(context.Background(), file, nil)
	case version:
		return c.Version(context.Background())
	case parsers:
		p, err := c.Parsers(context.Background())
		if err != nil {
			return "", err
		}
		bytes, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	case mimeTypes:
		mt, err := c.MIMETypes(context.Background())
		if err != nil {
			return "", err
		}
		bytes, err := json.MarshalIndent(mt, "", "  ")
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	case detectors:
		d, err := c.Detectors(context.Background())
		if err != nil {
			return "", err
		}
		bytes, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	}
}
