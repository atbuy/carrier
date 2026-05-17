package logs

import (
	_ "embed"

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

// BuiltinPatterns returns regexes from the embedded gitleaks rule set.
// Invalid regexes are silently skipped; callers compile them via NewRedactor.
func BuiltinPatterns() []string {
	var cfg gitleaksConfig
	if err := toml.Unmarshal(gitleaksToml, &cfg); err != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		if r.Regex != "" {
			out = append(out, r.Regex)
		}
	}
	return out
}
