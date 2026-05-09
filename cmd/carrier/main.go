package main

import (
	"os"

	"github.com/atbuy/carrier/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
