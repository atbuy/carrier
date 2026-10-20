package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	tableHeader = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$`)
	keyLine     = regexp.MustCompile(`^(\s*)([A-Za-z0-9_-]+)(\s*=\s*)(.*)$`)
)

// SetValue returns content with section.key set to value, a TOML literal such
// as `true` or `"30d"`. Only that line changes: an existing key keeps its
// indentation and trailing comment, a missing key is added at the end of its
// section, and a missing section is added at the end of the file. Comments,
// ordering, and formatting elsewhere are left as they are.
//
// The result is parsed to confirm it is valid TOML and that section.key now
// holds value; otherwise an error is returned and content should be kept.
func SetValue(content []byte, section, key, value string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	trailingNewline := len(lines) > 0 && lines[len(lines)-1] == ""
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}

	start, end := -1, len(lines) // section body is lines[start:end]
	current := ""
	for i, line := range lines {
		m := tableHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if current == section && start >= 0 {
			end = i
			break
		}
		current = strings.TrimSpace(m[1])
		if current == section && !strings.HasPrefix(strings.TrimSpace(line), "[[") {
			start = i + 1
		}
	}

	var out []string
	switch {
	case start < 0:
		out = append(out, lines...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, "["+section+"]", key+" = "+value)
	default:
		replaced := false
		for i := start; i < end; i++ {
			m := keyLine.FindStringSubmatch(lines[i])
			if m == nil || m[2] != key {
				continue
			}
			lines[i] = m[1] + m[2] + m[3] + value + trailingComment(m[4])
			replaced = true
			break
		}
		if replaced {
			out = lines
			break
		}
		// Insert after the section's last non-blank line.
		insert := start
		for i := start; i < end; i++ {
			if strings.TrimSpace(lines[i]) != "" {
				insert = i + 1
			}
		}
		out = append(out, lines[:insert]...)
		out = append(out, key+" = "+value)
		out = append(out, lines[insert:]...)
	}

	result := strings.Join(out, "\n")
	if trailingNewline || start < 0 {
		result += "\n"
	}
	if err := checkValue([]byte(result), section, key, value); err != nil {
		return nil, err
	}
	return []byte(result), nil
}

// trailingComment returns what follows a TOML value on its line: the
// comment with the spacing before it, exactly as written. A '#' inside a
// quoted string value is not a comment.
func trailingComment(rest string) string {
	if rest != "" && (rest[0] == '"' || rest[0] == '\'') {
		closing := strings.IndexByte(rest[1:], rest[0])
		if closing < 0 {
			return ""
		}
		rest = rest[closing+2:]
	}
	i := strings.Index(rest, "#")
	if i < 0 {
		return ""
	}
	j := i
	for j > 0 && (rest[j-1] == ' ' || rest[j-1] == '\t') {
		j--
	}
	if j == i {
		return " " + rest[i:]
	}
	return rest[j:]
}

// checkValue confirms content is valid TOML in which section.key decodes to
// the same value as the literal value.
func checkValue(content []byte, section, key, value string) error {
	var doc map[string]any
	if _, err := toml.NewDecoder(bytes.NewReader(content)).Decode(&doc); err != nil {
		return fmt.Errorf("edit would make the config invalid: %w", err)
	}
	var want map[string]any
	if _, err := toml.Decode("v = "+value, &want); err != nil {
		return fmt.Errorf("invalid value %s: %w", value, err)
	}
	table, ok := doc[section].(map[string]any)
	if !ok || fmt.Sprint(table[key]) != fmt.Sprint(want["v"]) {
		return fmt.Errorf("could not set %s.%s safely; edit the config by hand", section, key)
	}
	return nil
}

// DefinedIn returns the file whose value of section.key takes effect: the
// last include that defines it, else the main config. Values in includes
// override the main config, so editing the main file would have no effect
// when an include sets the key.
func DefinedIn(mainPath, section, key string) (string, error) {
	var main Config
	if _, err := toml.DecodeFile(mainPath, &main); err != nil {
		return "", err
	}
	includes, err := IncludeFiles(main.Includes, filepath.Dir(mainPath))
	if err != nil {
		return "", err
	}
	for i := len(includes) - 1; i >= 0; i-- {
		var overlay Config
		incMeta, err := toml.DecodeFile(includes[i], &overlay)
		if err != nil {
			return "", fmt.Errorf("includes: %s: %w", includes[i], err)
		}
		if incMeta.IsDefined(section, key) {
			return includes[i], nil
		}
	}
	return mainPath, nil
}
