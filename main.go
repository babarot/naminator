package main

import (
	"os"

	"github.com/babarot/naminator/internal/cli"
)

var (
	version  = "develop"
	revision = "HEAD"
)

func main() {
	os.Exit(cli.Main(version, revision))
}
