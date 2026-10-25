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

	start, end := sectionBody(lines, section)

	var out []string
	switch {
	case start < 0:
		out = append(out, lines...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, "["+section+"]", key+" = "+value)
	default:
		if i, last, rest, found := findKey(lines, start, end, key); found {
			m := keyLine.FindStringSubmatch(lines[i])
			out = append(out, lines[:i]...)
			out = append(out, m[1]+m[2]+m[3]+value+trailingComment(rest))
			out = append(out, lines[last+1:]...)
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

// sectionBody returns the lines of section's table as lines[start:end];
// start is -1 when the file has no such table.
func sectionBody(lines []string, section string) (start, end int) {
	start, end = -1, len(lines)
	current := ""
	for i, line := range lines {
		m := tableHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if current == section && start >= 0 {
			return start, i
		}
		current = strings.TrimSpace(m[1])
		if current == section && !strings.HasPrefix(strings.TrimSpace(line), "[[") {
			start = i + 1
		}
	}
	return start, end
}

// findKey finds key among lines[start:end]. Its value runs from line i to
// line last, as an array written over several lines does; rest is what
// follows "key = " on line i when the value fits on it, else what follows the
// closing bracket on line last, so a trailing comment can be kept.
func findKey(lines []string, start, end int, key string) (i, last int, rest string, found bool) {
	for i = start; i < end; i++ {
		m := keyLine.FindStringSubmatch(lines[i])
		if m == nil || m[2] != key {
			continue
		}
		if !strings.HasPrefix(m[4], "[") {
			return i, i, m[4], true
		}
		depth := 0
		var quote byte
		for last = i; last < end; last++ {
			line := lines[last]
			if last == i {
				line = m[4]
			}
			for j := 0; j < len(line); j++ {
				c := line[j]
				switch {
				case quote != 0:
					if c == '\\' && quote == '"' {
						j++
					} else if c == quote {
						quote = 0
					}
				case c == '"' || c == '\'':
					quote = c
				case c == '#':
					j = len(line)
				case c == '[':
					depth++
				case c == ']':
					depth--
					if depth == 0 {
						return i, last, line[j+1:], true
					}
				}
			}
		}
		return i, i, m[4], true
	}
	return 0, 0, "", false
}

// RemoveValue returns content without section.key, so the setting falls back
// to its default or to an earlier file. Only the key's lines change. Content
// without the key is returned as it is.
func RemoveValue(content []byte, section, key string) []byte {
	lines := strings.Split(string(content), "\n")
	start, end := sectionBody(lines, section)
	if start < 0 {
		return content
	}
	i, last, _, found := findKey(lines, start, end, key)
	if !found {
		return content
	}
	out := append(append([]string{}, lines[:i]...), lines[last+1:]...)
	return []byte(strings.Join(out, "\n"))
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
	table, ok := map[string]any(doc), true
	for _, part := range strings.Split(section, ".") {
		if table, ok = table[part].(map[string]any); !ok {
			break
		}
	}
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
		if incMeta.IsDefined(append(strings.Split(section, "."), key)...) {
			return includes[i], nil
		}
	}
	return mainPath, nil
}
