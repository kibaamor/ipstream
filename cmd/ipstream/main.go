// Package main provides the ipstream command.
package main

import (
	"errors"
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

	var showVersion bool

	fs.BoolVar(&showVersion, "v", false, "show version")
	fs.BoolVar(&showVersion, "version", false, "show version")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
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
	emitter := ipLineEmitter{
		out:     out,
		newline: []byte{'\n'},
	}
	streamer := ipstream.NewStreamer(&emitter)

	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(streamer.Writer(), in, buf); err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	streamer.Flush()
	return emitter.Err()
}

type ipLineEmitter struct {
	out     io.Writer
	newline []byte
	err     error
}

func (e *ipLineEmitter) Handle(raw []byte, addr netip.Addr) {
	if !addr.IsValid() || e.err != nil {
		return
	}
	if _, err := e.out.Write(raw); err != nil {
		e.err = err
		return
	}
	_, e.err = e.out.Write(e.newline)
}

func (e *ipLineEmitter) Err() error {
	if e.err == nil {
		return nil
	}
	return fmt.Errorf("write output: %w", e.err)
}
