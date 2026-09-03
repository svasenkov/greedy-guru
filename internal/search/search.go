package search

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type Result struct {
	OK    bool   `json:"ok"`
	Query string `json:"query"`
	Path  string `json:"path"`
	Hits  []Hit  `json:"hits"`
	Error string `json:"error,omitempty"`
}

func Run(ctx context.Context, query, path, glob string) (Result, int) {
	if query == "" {
		return Result{OK: false, Hits: []Hit{}, Error: "search: query required"}, 2
	}
	if path == "" {
		path = "."
	}
	rg, err := exec.LookPath("rg")
	if err != nil {
		return Result{OK: false, Query: query, Path: path, Hits: []Hit{}, Error: "search: rg not found in PATH"}, 2
	}
	args := []string{"-n", "--no-heading", "--with-filename", "--max-count", "50"}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", query, path)
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, rg, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	hits := parseHits(stdout.String())
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return Result{OK: false, Query: query, Path: path, Hits: hits}, 1
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{OK: false, Query: query, Path: path, Hits: hits, Error: msg}, 1
	}
	return Result{OK: true, Query: query, Path: path, Hits: hits}, 0
}

func parseHits(out string) []Hit {
	hits := []Hit{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		path, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		lineStr, text, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(lineStr)
		if err != nil {
			continue
		}
		hits = append(hits, Hit{Path: filepath.ToSlash(path), Line: n, Text: text})
	}
	return hits
}
