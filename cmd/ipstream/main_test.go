//go:build ipstreamtests
// +build ipstreamtests

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args []string, input string) (code int, stdout string, stderr string) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code = run(args, strings.NewReader(input), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun_FiltersIPsToStdout(t *testing.T) {
	input := "hello 192.168.1.1 ::1 world 999.1.1.1 fe80::1%1 ::1%eth0 ::1%eth0.1!"

	code, stdout, stderr := runCLI(t, nil, input)
	if code != 0 {
		t.Fatalf("run() code=%d, stderr=%q", code, stderr)
	}

	want := "192.168.1.1\n::1\nfe80::1%1\n::1%eth0\n::1%eth0.1\n"
	if stdout != want {
		t.Fatalf("stdout=%q, want %q", stdout, want)
	}
}

func TestRun_DrainsFinalToken(t *testing.T) {
	code, stdout, stderr := runCLI(t, nil, "prefix 2001:db8::1")
	if code != 0 {
		t.Fatalf("run() code=%d, stderr=%q", code, stderr)
	}

	if stdout != "2001:db8::1\n" {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestRun_Help(t *testing.T) {
	code, stdout, stderr := runCLI(t, []string{"-h"}, "ignored")
	if code != 0 {
		t.Fatalf("run() code=%d, want 0, stderr=%q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout=%q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Usage: ipstream") {
		t.Fatalf("stderr=%q, want usage", stderr)
	}
}

func TestRun_Version(t *testing.T) {
	code, stdout, stderr := runCLI(t, []string{"-v"}, "ignored")
	if code != 0 {
		t.Fatalf("run() code=%d, want 0, stderr=%q", code, stderr)
	}
	want := "Version: " + version + "\n" +
		"Commit: " + gitCommit + "\n" +
		"Build Date: " + buildDate + "\n" +
		"Home Page: " + homePage + "\n" +
		"Author: " + author + "\n" +
		"License: " + license + "\n"
	if stdout != want {
		t.Fatalf("stdout=%q, want %q", stdout, want)
	}
}

func TestRun_RejectsArgs(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"file.log"}, "ignored")
	if code != 2 {
		t.Fatalf("run() code=%d, want 2", code)
	}
	if !strings.Contains(stderr, "unexpected arguments") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRun_RejectsInvalidFlag(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"--bad"}, "ignored")
	if code != 2 {
		t.Fatalf("run() code=%d, want 2", code)
	}
	if !strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRun_ReportsFilterError(t *testing.T) {
	code := run(nil, errorReader{err: errors.New("boom")}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 1 {
		t.Fatalf("run() code=%d, want 1", code)
	}
}

func TestFilterIPs_StopsReadingAfterWriteError(t *testing.T) {
	reader := &chunkReader{chunks: []string{"1.2.3.4 ", "5.6.7.8 "}}
	writer := &errorWriter{err: errors.New("boom")}

	err := filterIPs(reader, writer)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "write output: boom") {
		t.Fatalf("err=%v", err)
	}
	if reader.reads != 1 {
		t.Fatalf("reads=%d, want 1", reader.reads)
	}
}

func TestFilterIPs_ReportsFlushWriteError(t *testing.T) {
	err := filterIPs(strings.NewReader("1.2.3.4"), &errorWriter{err: errors.New("boom")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "write output: boom") {
		t.Fatalf("err=%v", err)
	}
}

type chunkReader struct {
	chunks []string
	reads  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	r.reads++
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

type errorReader struct {
	err error
}

func (r errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}
