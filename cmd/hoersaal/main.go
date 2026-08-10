// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command hoersaal is the service entry point. It reads the configuration and
// then does nothing, because nothing else is implemented yet.
//
// It exists so that the toolchain decision on issue #15 is proved by a build
// rather than asserted, and so that the checks on this milestone have something
// to compile. What the repository is laid out as, and where the boundary
// between the control plane and the media plane falls in it, is issue #16.
//
// Reading the configuration is here rather than in internal/config because
// docs/repository-layout.md puts it here: a command holds the wiring and
// decides nothing, and the whole of what a key means, what its default is and
// what makes a value invalid is in that package, where the suite exercises each
// refusal against a string rather than against a file somebody has to create.
//
// The order is the point of issue #82's third condition. The configuration is
// read and judged before anything the settings describe is constructed, so a
// deployment with a mistake in its file stops at the mistake rather than during
// the first lecture. Today that order is cheap to keep, because there is
// nothing after it: this process listens on no socket, opens no store and
// reaches no unit. It is written in this order now so that the thing that
// listens arrives underneath a refusal that already happened.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iderex/hoersaal/internal/config"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is main with its edges handed in, so that the suite can start this
// command with an argument list and read what it wrote. A main that called
// os.Exit directly would be a startup nothing could assert, and the refusal
// this command exists to make is exactly the thing worth asserting.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("hoersaal", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "",
		"the configuration file to read; with no path the defaults are used, which is the same as an empty file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "hoersaal: %q is not an argument this command takes\n", flags.Arg(0))
		return 2
	}

	settings, err := load(*path)
	if err != nil {
		fmt.Fprintf(stderr, "hoersaal: %v\n", err)
		return 2
	}

	fmt.Fprintf(stdout,
		"hoersaal: the configuration is valid; the pool floor is %d and the ceiling is %d, and nothing is implemented yet\n",
		settings.PoolMinimum, settings.PoolMaximum)
	return 0
}

// load answers with the settings the operator gave, or the defaults where they
// named no file. An absent path and an empty file are deliberately the same
// answer: issue #82 asks that the service start with an empty configuration
// file, and a deployment that has not written one yet is in the same position.
func load(path string) (config.Settings, error) {
	if path == "" {
		return config.Load(strings.NewReader(""))
	}

	// #nosec G304 -- the path is the one whoever started this command named on
	// its command line, and the file it opens is that operator's own
	// configuration. There is no untrusted path here to constrain.
	f, err := os.Open(path)
	if err != nil {
		return config.Settings{}, fmt.Errorf("opening the configuration: %w", err)
	}
	defer func() { _ = f.Close() }()

	return config.Load(f)
}
