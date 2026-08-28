// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"bytes"
	"log"
	"regexp"
	"testing"
)

func TestTimestampWriterFormat(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(timestampWriter{w: &buf}, "", 0)
	l.Print("hello")

	re := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3} hello\n$`)
	if !re.MatchString(buf.String()) {
		t.Fatalf("unexpected log line: %q", buf.String())
	}
}
