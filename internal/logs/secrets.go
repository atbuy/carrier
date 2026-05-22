package logs

import (
	_ "embed"
	"sync"

	"github.com/BurntSushi/toml"
)

//go:embed gitleaks.toml
var gitleaksToml []byte

type gitleaksConfig struct {
	Rules []gitleaksRule `toml:"rules"`
}

type gitleaksRule struct {
	Regex string `toml:"regex"`
}

var (
	builtinOnce     sync.Once
	builtinPatterns []string
)

// BuiltinPatterns returns regexes from the embedded gitleaks rule set.
// The result is parsed once and cached for the lifetime of the process.
// Invalid regexes are silently skipped; callers compile them via NewRedactor.
func BuiltinPatterns() []string {
	builtinOnce.Do(func() {
		var cfg gitleaksConfig
		if err := toml.Unmarshal(gitleaksToml, &cfg); err != nil {
			return
		}
		out := make([]string, 0, len(cfg.Rules))
		for _, r := range cfg.Rules {
			if r.Regex != "" {
				out = append(out, r.Regex)
			}
		}
		builtinPatterns = out[:len(out):len(out)]
	})
	return builtinPatterns
}
