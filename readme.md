# go-tika

go-tika is a Go client library and command line utility for accessing the [Apache Tika](http://tika.apache.org) Server API.

go-tika requires Go version 1.7 or greater.

## Usage
Package `tika` provides a client and server for downloading, starting, and using the [Apache Tika](http://tika.apache.org) Server API.

Start with basic imports:

```go
import (
	"os"

	"github.com/google/go-tika/tika"
)
```

If you don't have a downloaded Tika Server JAR, you can download one:

```go
err := tika.DownloadServer("1.14", "tika-server-1.14.jar")
if err != nil {
	log.Fatal(err)
}
```

If you don't have a running Tika Server, you can start one:

```go
s, err := tika.StartServer("tika-server-1.14.jar", nil)
if err != nil {
	log.Fatal(err)
}
defer s.Shutdown()
```

Open any io.Reader:

```go
f, err := os.Open("path/to/file")
if err != nil {
	log.Fatal(err)
}
defer f.Close()
```

Create a client and parse the io.Reader:

```go
client := tika.NewClient(nil, s.URL())
body, err := client.Parse(f)
```

If you pass an `*http.Client` to `tika.NewClient`, it will be used for all requests.

Some functions return a custom type, like a `Parsers()`, `Detectors()`, and `MimeTypes()`:

```go
parsers, err := client.Parsers()
detectors, err := client.Detectors()
mimeTypes, err := client.MimeTypes()
```

See [the godoc](https://godoc.org/github.com/google/go-tika/tika) for more documentation on what resources are available.

## Command line client

The `tika` binary allows you to access the Apache Tika Server API from the command line, including downloading and starting the server in the background.

To get the binary, run:

```bash
go get github.com/google/go-tika/cmd/tika
```

To download the Apache Tika 1.14 Server, check the MD5 sum, start the server in the background, and parse a file, run:

```bash
$GOPATH/bin/tika -action parse -filename /path/to/file -downloadVersion 1.14
```

This will store tika-server-1.14.jar in your current working directory. If you want to control the output location of the JAR, add a `-serverJAR /path/to/save/tika-server.jar` argument.

If you already have a downloaded Apache Tika Server JAR, you can specify it with the `-serverJAR` flag and it will not be re-downloaded.

If you already have a running Apache Tika Server, you can use it by adding the `-serverURL` flag and omitting the `-serverJAR` and `-downloadVersion` flags.

See `$GOPATH/bin/tika -h` for usage instructions.

## License

This library is distributed under the Apache V2 License. See the [LICENSE](./LICENSE) file.

## Contributing

Please see the [CONTRIBUTING.md](./CONTRIBUTING.md) file.

## Disclaimer

This is not an official Google product.
