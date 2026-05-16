// Package main provides the ipstream command.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"

	"github.com/kibaamor/ipstream"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

const (
	homePage = "github.com/kibaamor/ipstream"
	author   = "Kiba Amor"
	license  = "Apache-2.0"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("ipstream", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		_, _ = fmt.Fprint(errOut, `Usage: ipstream [OPTION]...

Extract IPv4 and IPv6 addresses from standard input, one per line.

  -h, --help                        display this help and exit
  -v, --version                     output version information and exit
`)
	}

	var showHelp bool
	var showVersion bool

	fs.BoolVar(&showHelp, "h", false, "show help")
	fs.BoolVar(&showHelp, "help", false, "show help")
	fs.BoolVar(&showVersion, "v", false, "show version")
	fs.BoolVar(&showVersion, "version", false, "show version")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if showHelp {
		fs.Usage()
		return 0
	}
	if showVersion {
		_, _ = fmt.Fprintf(out, "Version: %s\nCommit: %s\nBuild Date: %s\nHome Page: %s\nAuthor: %s\nLicense: %s\n",
			version, gitCommit, buildDate, homePage, author, license)
		return 0
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(errOut, "ipstream: unexpected arguments: %v\n", fs.Args())
		fs.Usage()
		return 2
	}
	if err := filterIPs(in, out); err != nil {
		_, _ = fmt.Fprintf(errOut, "ipstream: %v\n", err)
		return 1
	}
	return 0
}

func filterIPs(in io.Reader, out io.Writer) error {
	var writeErr error

	newline := []byte{'\n'}
	streamer := ipstream.NewStreamer(ipstream.HandleFunc(func(raw []byte, addr netip.Addr) {
		if !addr.IsValid() || writeErr != nil {
			return
		}
		if _, err := out.Write(raw); err != nil {
			writeErr = err
			return
		}
		_, writeErr = out.Write(newline)
	}))

	if _, err := io.Copy(streamer, stopOnWriteErrorReader{r: in, err: &writeErr}); err != nil {
		if writeErr != nil {
			return fmt.Errorf("write output: %w", writeErr)
		}
		return fmt.Errorf("read input: %w", err)
	}
	_ = streamer.Close()
	if writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	return nil
}

type stopOnWriteErrorReader struct {
	r   io.Reader
	err *error
}

func (r stopOnWriteErrorReader) Read(p []byte) (int, error) {
	if *r.err != nil {
		return 0, *r.err
	}
	return r.r.Read(p)
}
