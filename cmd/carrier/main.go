package main

import (
	"os"

	"github.com/user/carrier/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
