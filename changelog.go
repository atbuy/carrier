// Package carrier holds files from the repository root that are built into
// the carrier binary.
package carrier

import _ "embed"

// Changelog is CHANGELOG.md, shown by "carrier self changelog".
//
//go:embed CHANGELOG.md
var Changelog string
