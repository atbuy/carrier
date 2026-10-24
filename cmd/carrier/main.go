package main

import (
	"os"

	"github.com/atbuy/carrier/internal/cli"
	_ "github.com/atbuy/carrier/internal/termbg"
)

// internal/termbg is imported for its init, which keeps Bubble Tea from
// asking the terminal for its background color at startup.
func main() {
	os.Exit(cli.Execute())
}
