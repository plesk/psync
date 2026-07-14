// Copyright 1999-2026. WebPros International GmbH.

package main

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/plesk/psync/cmd"
)

var (
	commit = "000000"
	date   = ""
)

//go:embed version
var version string

func init() {
	fullVersion := strings.TrimSpace(version)
	if date != "" {
		fullVersion += fmt.Sprintf(" (%s, %s)", date, commit)
	}
	cmd.Version = fullVersion
}

func main() {
	cmd.Execute()
}
