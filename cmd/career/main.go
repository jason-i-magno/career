// Command career is a job-search cockpit: an application pipeline, a STAR story
// bank, and a spaced-repetition interview prep deck in one local-first CLI.
package main

import (
	"os"

	"github.com/jason-i-magno/career/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
