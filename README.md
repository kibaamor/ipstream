# ipstream

`ipstream` extracts IPv4 and IPv6 addresses from byte streams.

## CLI

The `ipstream` command extracts IPv4 and IPv6 addresses from standard input, one per line.

### Install

#### Download prebuilt binaries

Download an archive from [GitHub Releases](https://github.com/kibaamor/ipstream/releases/latest).

Optionally, download `checksums.txt` and verify the archive:

```bash
cd path/to/downloads
sha256sum -c checksums.txt --ignore-missing
```

Extract the archive and place `ipstream` in your `PATH`.

#### Package managers

Release builds are configured for Homebrew, Scoop, WinGet, Snapcraft, NUR, Linux distro packages (`deb`, `rpm`, `apk`, `ipk`, Arch Linux), and GHCR container images.

```bash
brew install kibaamor/tap/ipstream
scoop bucket add kibaamor https://github.com/kibaamor/scoop-bucket
scoop install ipstream
winget install KibaAmor.ipstream
snap install ipstream
docker run --rm -i ghcr.io/kibaamor/ipstream:latest < input.log
```

#### Install from source

```bash
go install github.com/kibaamor/ipstream/cmd/ipstream@latest
```

### Usage

```bash
ipstream < input.log
```

Examples:

```bash
# Extract IPs from a log.
ipstream < input.log

# Show help.
ipstream -h

# Show version metadata.
ipstream -v
```

Useful flags: `-h/--help`, `-v/--version`.

## Go Library

The Go library scans arbitrary chunks and reports both IP and non-IP segments to a handler.

### Install

```bash
go get github.com/kibaamor/ipstream
```

### Usage

Use `NewStreamer` to scan arbitrary chunks. The handler receives every input segment; `addr.IsValid()` means `raw` parsed as `addr`.

```go
package main

import (
	"fmt"
	"net/netip"

	"github.com/kibaamor/ipstream"
)

func main() {
	streamer := ipstream.NewStreamer(ipstream.HandleFunc(func(raw []byte, addr netip.Addr) {
		if addr.IsValid() {
			fmt.Println(addr)
		}
	}))

	_, _ = streamer.Write([]byte("client=192.168.1.1 "))
	_, _ = streamer.Write([]byte("gateway=2001:db8::1"))
	_ = streamer.Close() // flushes the final token
}
```

Output:

```text
192.168.1.1
2001:db8::1
```

## Issues

Bug reports and feature suggestions are welcome in [GitHub Issues](https://github.com/kibaamor/ipstream/issues).

When reporting a bug, please include the input that reproduces it, the expected output, the actual output, and the `ipstream -v` version information if you are using the CLI.

## License

Apache License 2.0. See [LICENSE](./LICENSE).
