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

To download the Apache Tika 3.3.2 Server, check the SHA-512 sum, start the server in the background, and parse a file, run:

```bash
$(go env GOPATH)/bin/tika -filename /path/to/file/to/parse -download_version 3.3.2 parse
```

This will store `tika-server-3.3.2.jar` in your current working directory. If you want to control the output location of the JAR, add a `-server_jar /path/to/save/tika-server.jar` argument.

If you already have a downloaded Apache Tika Server JAR, you can specify it with the `-server_jar` flag and it will not be re-downloaded.

To configure the server that is started (for example, forking, timeouts or the maximum number of files per child process), pass a [tika-config.xml](https://cwiki.apache.org/confluence/display/TIKA/TikaServer) file with the `-server_config` flag. When using the library, set `Server.ConfigPath` instead.

If you already have a running Apache Tika Server, you can use it by adding the `-server_url` flag and omitting the `-server_jar` and `-download_version` flags.

See `$(go env GOPATH)/bin/tika -h` for usage instructions.

## Supported Tika Server versions

go-tika can download and run Apache Tika Server 2.6.0, 2.7.0, 2.8.0, 2.9.4, 3.0.0, 3.1.0, 3.2.3 and 3.3.2. Tika Server 2.x requires Java 8 or later, and 3.x requires Java 11 or later.

Tika Server 1.19, 1.20 and 1.21 are deprecated: Tika 1.x is end-of-life.

## License

This library is distributed under the Apache V2 License. See the [LICENSE](./LICENSE) file.

## Contributing

Please see the [CONTRIBUTING.md](./CONTRIBUTING.md) file.

Use `goimports` to format code and make sure the `go.mod`/`go.sum` files are up to date with `go mod tidy`.

## Disclaimer

This is not an official Google product.
