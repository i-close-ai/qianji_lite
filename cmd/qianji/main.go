package main

import (
	"os"

	"github.com/i-close-ai/qianji_lite/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
