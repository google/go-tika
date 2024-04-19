# go-tika

[![Go Reference](https://pkg.go.dev/badge/github.com/google/go-tika.svg)](https://pkg.go.dev/github.com/google/go-tika)

go-tika is a Go client library and command line utility for accessing the [Apache Tika](http://tika.apache.org) Server API.

See https://pkg.go.dev/github.com/google/go-tika for more documentation on what resources are available.

## Command line client

The `tika` binary allows you to access the Apache Tika Server API from the command line, including downloading and starting the server in the background.

To get the binary, run:

```bash
go install github.com/google/go-tika/cmd/tika@latest
```

To download the Apache Tika 1.14 Server, check the MD5 sum, start the server in the background, and parse a file, run:

```bash
$(go env GOPATH)/bin/tika -filename /path/to/file/to/parse -download_version 1.14 parse
```

This will store `tika-server-1.14.jar` in your current working directory. If you want to control the output location of the JAR, add a `-server_jar /path/to/save/tika-server.jar` argument.

If you already have a downloaded Apache Tika Server JAR, you can specify it with the `-server_jar` flag and it will not be re-downloaded.

If you already have a running Apache Tika Server, you can use it by adding the `-server_url` flag and omitting the `-server_jar` and `-download_version` flags.

See `$(go env GOPATH)/bin/tika -h` for usage instructions.

## License

This library is distributed under the Apache V2 License. See the [LICENSE](./LICENSE) file.

## Contributing

Please see the [CONTRIBUTING.md](./CONTRIBUTING.md) file.

Use `goimports` to format code and make sure the `go.mod`/`go.sum` files are up to date with `go mod tidy`.

## Disclaimer

This is not an official Google product.
