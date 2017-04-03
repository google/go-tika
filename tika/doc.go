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

/*
Package tika provides a client and server for downloading, starting, and using Apache Tika's (http://tika.apache.org) Server.

Start with basic imports:

	import (
		"os"

		"github.com/google/go-tika/tika"
	)

If you don't have a downloaded Tika Server JAR, you can download one.

	err := tika.DownloadServer("1.14", "tika-server-1.14.jar")
	if err != nil {
		log.Fatal(err)
	}

If you don't have a running Tika Server, you can start one.

	s, err := tika.StartServer("tika-server-1.14.jar", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Shutdown()

Pass a *tika.ServerConfig to control the Server's behavior.

Open any io.Reader.

	f, err := os.Open("path/to/file")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

Create a client and parse the io.Reader.

	client := tika.NewClient(nil, s.URL())
	body, err := client.Parse(f)

If you pass an *http.Client to tika.NewClient, it will be used for all requests.

Some functions return a custom type, like a Parsers(), Detectors(), and
MimeTypes():

	parsers, err := client.Parsers()
	detectors, err := client.Detectors()
	mimeTypes, err := client.MimeTypes()
*/
package tika
