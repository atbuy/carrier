package logs

import (
	"math"
	"net/url"
	"strings"
	"unicode"
)

var sensitiveEnvExactKeys = map[string]struct{}{
	"ACCESS_TOKEN":                   {},
	"AWS_ACCESS_KEY_ID":              {},
	"AWS_SECRET_ACCESS_KEY":          {},
	"AWS_SESSION_TOKEN":              {},
	"AZURE_CLIENT_SECRET":            {},
	"AZURE_STORAGE_KEY":              {},
	"DB_PASS":                        {},
	"DB_PASSWORD":                    {},
	"GCP_SERVICE_ACCOUNT_KEY":        {},
	"GH_TOKEN":                       {},
	"GITHUB_TOKEN":                   {},
	"GITLAB_TOKEN":                   {},
	"GOOGLE_APPLICATION_CREDENTIALS": {},
	"HEROKU_API_KEY":                 {},
	"NPM_TOKEN":                      {},
	"OPENAI_API_KEY":                 {},
	"PGPASSWORD":                     {},
	"PG_PASSWORD":                    {},
	"SENDGRID_API_KEY":               {},
	"SENDGRID_KEY":                   {},
	"STRIPE_SECRET_KEY":              {},
	"STRIPE_SK":                      {},
	"TWILIO_AUTH_TOKEN":              {},
	"VAULT_TOKEN":                    {},
}

var sensitiveEnvTokens = map[string]struct{}{
	"ACCESSKEY":      {},
	"ACCESSTOKEN":    {},
	"APIKEY":         {},
	"AUTH":           {},
	"AUTHTOKEN":      {},
	"CERT":           {},
	"CERTIFICATE":    {},
	"CLIENTSECRET":   {},
	"CREDENTIAL":     {},
	"CREDENTIALS":    {},
	"KEY":            {},
	"PASSPHRASE":     {},
	"PASSWD":         {},
	"PASSWORD":       {},
	"PEM":            {},
	"PRIVATEKEY":     {},
	"SECRET":         {},
	"SECRETKEY":      {},
	"SERVICEACCOUNT": {},
	"SIGNINGKEY":     {},
	"TOKEN":          {},
}

var sensitiveEnvProviderTokens = map[string]struct{}{
	"ADAFRUIT":     {},
	"ALGOLIA":      {},
	"ANTHROPIC":    {},
	"ARTIFACTORY":  {},
	"ATLASSIAN":    {},
	"AWS":          {},
	"AZURE":        {},
	"CLOUDFLARE":   {},
	"DATABRICKS":   {},
	"DIGITALOCEAN": {},
	"DISCORD":      {},
	"DOCKER":       {},
	"DROPBOX":      {},
	"GCP":          {},
	"GITHUB":       {},
	"GITLAB":       {},
	"GOOGLE":       {},
	"HEROKU":       {},
	"HUGGINGFACE":  {},
	"LINEAR":       {},
	"MAILCHIMP":    {},
	"MAILGUN":      {},
	"MONGODB":      {},
	"NOTION":       {},
	"NPM":          {},
	"OPENAI":       {},
	"POSTMARK":     {},
	"REDIS":        {},
	"SENDGRID":     {},
	"SENTRY":       {},
	"SLACK":        {},
	"STRIPE":       {},
	"SUPABASE":     {},
	"TWILIO":       {},
	"VERCEL":       {},
	"VAULT":        {},
}

var sensitiveEnvProviderSecretTokens = map[string]struct{}{
	"KEY":    {},
	"SECRET": {},
	"SK":     {},
	"TOKEN":  {},
}

func sensitiveEnvKey(key string) bool {
	normalized := normalizeEnvKey(key)
	if normalized == "" {
		return false
	}
	if _, ok := sensitiveEnvExactKeys[normalized]; ok {
		return true
	}

	tokens := strings.Split(normalized, "_")
	for _, token := range tokens {
		if _, ok := sensitiveEnvTokens[token]; ok {
			return true
		}
	}

	for i, token := range tokens {
		if _, ok := sensitiveEnvProviderTokens[token]; !ok {
			continue
		}
		for _, next := range tokens[i+1:] {
			if _, ok := sensitiveEnvProviderSecretTokens[next]; ok {
				return true
			}
		}
	}
	return false
}

func sensitiveEnvValue(key, value string) bool {
	if value == "" {
		return false
	}
	if envValueCanContainCredentials(key) && urlContainsCredentials(value) {
		return true
	}
	return looksLikeHighEntropySecret(value)
}

func normalizeEnvKey(key string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func envValueCanContainCredentials(key string) bool {
	normalized := normalizeEnvKey(key)
	for _, suffix := range []string{"URL", "URI", "DSN", "DATABASE"} {
		if normalized == suffix || strings.HasSuffix(normalized, "_"+suffix) {
			return true
		}
	}
	return false
}

func urlContainsCredentials(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User == nil {
		return false
	}
	_, hasPassword := u.User.Password()
	return hasPassword || u.User.Username() != ""
}

func looksLikeHighEntropySecret(value string) bool {
	if len(value) < 24 || strings.ContainsAny(value, " \t\r\n\\/") || strings.Contains(value, ":") {
		return false
	}
	var alpha, digit, symbol int
	for _, r := range value {
		switch {
		case unicode.IsLetter(r):
			alpha++
		case unicode.IsDigit(r):
			digit++
		default:
			symbol++
		}
	}
	if alpha == 0 || digit == 0 || alpha+digit+symbol < 24 {
		return false
	}
	return shannonEntropy(value) >= 4.2
}

func shannonEntropy(s string) float64 {
	counts := make(map[rune]int, len(s))
	total := 0
	for _, r := range s {
		counts[r]++
		total++
	}
	var entropy float64
	for _, count := range counts {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}
