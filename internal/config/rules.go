package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Rule edits change the [[auto_label]] tables of the main config file in
// place, like SetValue does for settings: a changed rule keeps its comments
// and the order of its keys, and the rest of the file stays as it is. Rules
// in includes are left to their files; they come after the main file's.

// ruleKeys are the keys of an auto_label rule, in the order a new rule is
// written.
var ruleKeys = []string{"label", "cmd", "dir", "git_branch"}

func (r AutoLabel) value(key string) string {
	switch key {
	case "label":
		return r.Label
	case "cmd":
		return r.Cmd
	case "dir":
		return r.Dir
	}
	return r.GitBranch
}

// Validate reports what is wrong with a rule, as "carrier config check"
// would: an empty label or a pattern that is not a regular expression.
func (r AutoLabel) Validate() error {
	if strings.TrimSpace(r.Label) == "" {
		return errors.New("the label must not be empty")
	}
	for _, key := range ruleKeys[1:] {
		if p := r.value(key); p != "" {
			if _, err := regexp.Compile(p); err != nil {
				return fmt.Errorf("%s is not a regular expression: %w", key, err)
			}
		}
	}
	return nil
}

// MainAutoLabels returns the rules written in the main config file, which
// "carrier config" edits, without those from includes.
func MainAutoLabels(mainPath string) ([]AutoLabel, error) {
	content, err := os.ReadFile(mainPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeRules(content)
}

func decodeRules(content []byte) ([]AutoLabel, error) {
	var c Config
	if _, err := toml.Decode(string(content), &c); err != nil {
		return nil, err
	}
	return c.AutoLabels, nil
}

// AddAutoLabel adds rule after the main config's rules, so it is tried last.
func AddAutoLabel(mainPath string, rule AutoLabel) error {
	return editRules(mainPath, func(lines []string, blocks []ruleBlock, rules []AutoLabel) ([]string, []AutoLabel, error) {
		text := ruleText(rule)
		at := len(lines)
		if len(blocks) > 0 {
			at = blocks[len(blocks)-1].end
		}
		// One blank line before the rule, and after it when more follows.
		for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		var insert []string
		if at > 0 {
			insert = append(insert, "")
		}
		insert = append(insert, text...)
		rest := lines[at:]
		for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
			rest = rest[1:]
		}
		if len(rest) > 0 {
			insert = append(insert, "")
		}
		out := append(append(append([]string{}, lines[:at]...), insert...), rest...)
		return out, append(slices.Clone(rules), rule), nil
	})
}

// SetAutoLabel replaces rule i of the main config. Keys it keeps keep their
// line and comment; emptied keys are removed and new ones added after the
// rule's last key.
func SetAutoLabel(mainPath string, i int, rule AutoLabel) error {
	return editRules(mainPath, func(lines []string, blocks []ruleBlock, rules []AutoLabel) ([]string, []AutoLabel, error) {
		if i < 0 || i >= len(blocks) {
			return nil, nil, fmt.Errorf("there is no rule %d", i+1)
		}
		b := blocks[i]
		body := slices.Clone(lines[b.header+1 : b.end])
		for _, key := range ruleKeys {
			// An unchanged key keeps its line as written.
			if rule.value(key) != rules[i].value(key) {
				body = setRuleKey(body, key, rule.value(key))
			}
		}
		out := append(append(append([]string{}, lines[:b.header+1]...), body...), lines[b.end:]...)
		updated := slices.Clone(rules)
		updated[i] = rule
		return out, updated, nil
	})
}

// RemoveAutoLabel removes rule i of the main config, with the comment lines
// right above it.
func RemoveAutoLabel(mainPath string, i int) error {
	return editRules(mainPath, func(lines []string, blocks []ruleBlock, rules []AutoLabel) ([]string, []AutoLabel, error) {
		if i < 0 || i >= len(blocks) {
			return nil, nil, fmt.Errorf("there is no rule %d", i+1)
		}
		b := blocks[i]
		from, to := b.from, b.end
		for to < len(lines) && strings.TrimSpace(lines[to]) == "" {
			to++
		}
		// Keep a blank line between what was around the rule.
		if from > 0 && to < len(lines) && strings.TrimSpace(lines[from-1]) != "" {
			to--
		}
		for to == len(lines) && from > 0 && strings.TrimSpace(lines[from-1]) == "" {
			from--
		}
		out := append(append([]string{}, lines[:from]...), lines[to:]...)
		return out, slices.Delete(slices.Clone(rules), i, i+1), nil
	})
}

// MoveAutoLabel swaps rule i of the main config with rule j, which is tried
// before or after it, with the comments right above each.
func MoveAutoLabel(mainPath string, i, j int) error {
	return editRules(mainPath, func(lines []string, blocks []ruleBlock, rules []AutoLabel) ([]string, []AutoLabel, error) {
		if i == j || i < 0 || j < 0 || i >= len(blocks) || j >= len(blocks) {
			return nil, nil, errors.New("the rule cannot move there")
		}
		a, b := blocks[min(i, j)], blocks[max(i, j)]
		out := append([]string{}, lines[:a.from]...)
		out = append(out, lines[b.from:b.end]...)
		out = append(out, lines[a.end:b.from]...)
		out = append(out, lines[a.from:a.end]...)
		out = append(out, lines[b.end:]...)
		updated := slices.Clone(rules)
		updated[i], updated[j] = updated[j], updated[i]
		return out, updated, nil
	})
}

// ruleBlock is where a rule is written: lines[from:end], from its comments
// to its last key, with its [[auto_label]] header at lines[header].
type ruleBlock struct{ from, header, end int }

func ruleBlocks(lines []string) []ruleBlock {
	var headers []int
	isRule := map[int]bool{}
	for i, line := range lines {
		if m := tableHeader.FindStringSubmatch(line); m != nil {
			headers = append(headers, i)
			isRule[i] = strings.HasPrefix(strings.TrimSpace(line), "[[") && strings.TrimSpace(m[1]) == "auto_label"
		}
	}
	var blocks []ruleBlock
	for n, h := range headers {
		if !isRule[h] {
			continue
		}
		end := len(lines)
		if n+1 < len(headers) {
			end = headers[n+1]
		}
		// Blank lines and comments after the last key belong to what follows.
		for end > h+1 && isBlankOrComment(lines[end-1]) {
			end--
		}
		from := h
		for from > 0 && strings.HasPrefix(strings.TrimSpace(lines[from-1]), "#") {
			from--
		}
		if len(blocks) > 0 {
			from = max(from, blocks[len(blocks)-1].end)
		}
		blocks = append(blocks, ruleBlock{from: from, header: h, end: end})
	}
	return blocks
}

func isBlankOrComment(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "#")
}

func ruleText(rule AutoLabel) []string {
	lines := []string{"[[auto_label]]"}
	for _, key := range ruleKeys {
		if v := rule.value(key); v != "" {
			lines = append(lines, key+" = "+quote(v))
		}
	}
	return lines
}

// setRuleKey sets key to value in a rule's body, or removes it when value is
// empty.
func setRuleKey(body []string, key, value string) []string {
	i, last, rest, found := findKey(body, 0, len(body), key)
	switch {
	case found && value == "":
		return append(append([]string{}, body[:i]...), body[last+1:]...)
	case found:
		m := keyLine.FindStringSubmatch(body[i])
		line := m[1] + m[2] + m[3] + quote(value) + trailingComment(rest)
		return append(append(append([]string{}, body[:i]...), line), body[last+1:]...)
	case value == "":
		return body
	}
	at := 0
	for j, line := range body {
		if !isBlankOrComment(line) {
			at = j + 1
		}
	}
	return append(append(append([]string{}, body[:at]...), key+" = "+quote(value)), body[at:]...)
}

// editRules applies edit to the main config's text, and writes the result
// when its rules decode to what edit says they should be and the config
// still loads.
func editRules(mainPath string, edit func(lines []string, blocks []ruleBlock, rules []AutoLabel) ([]string, []AutoLabel, error)) error {
	content, err := os.ReadFile(mainPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		content = []byte(NewFileHeader)
	}
	rules, err := decodeRules(content)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	trailingNewline := lines[len(lines)-1] == ""
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	blocks := ruleBlocks(lines)
	if len(blocks) != len(rules) {
		return fmt.Errorf("the auto_label rules in %s are not written as [[auto_label]] tables; edit them in the file", mainPath)
	}
	out, want, err := edit(lines, blocks, rules)
	if err != nil {
		return err
	}
	for i, rule := range want {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	updated := []byte(strings.Join(out, "\n") + "\n")
	got, err := decodeRules(updated)
	if err != nil || !sameRules(got, want) {
		return errors.New("could not change the rules safely; edit the config file by hand")
	}
	return write(mainPath, mainPath, updated, func(field string) bool { return strings.HasPrefix(field, "auto_label[") })
}

func sameRules(a, b []AutoLabel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		for _, key := range ruleKeys {
			if a[i].value(key) != b[i].value(key) {
				return false
			}
		}
	}
	return true
}
