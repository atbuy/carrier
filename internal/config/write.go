package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// NewFileHeader starts a config file that carrier creates to save a setting.
const NewFileHeader = "# carrier configuration. Run \"carrier config\" to change settings, or edit this file.\n"

// LoadFrom loads the config at path and its includes, like Load does for
// the config at Path.
func LoadFrom(path string) (Config, error) { return load(path, os.ReadFile) }

// Source returns the file whose value of s takes effect, or "" when no file
// sets it and the default applies. For a list, which includes add to, it is
// the main config when that sets the list, else the first include that does.
func Source(mainPath string, s Setting) (string, error) {
	files, err := configFiles(mainPath)
	if err != nil || len(files) == 0 {
		return "", err
	}
	if s.Kind == KindList {
		for _, file := range files {
			if defines(file, s) {
				return file, nil
			}
		}
		return "", nil
	}
	for i := len(files) - 1; i >= 0; i-- {
		if defines(files[i], s) {
			return files[i], nil
		}
	}
	return "", nil
}

// configFiles returns the main config and its includes, in merge order, or
// nothing when there is no config file.
func configFiles(mainPath string) ([]string, error) {
	var main Config
	if _, err := toml.DecodeFile(mainPath, &main); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	includes, err := IncludeFiles(main.Includes, filepath.Dir(mainPath))
	if err != nil {
		return nil, err
	}
	return append([]string{mainPath}, includes...), nil
}

func defines(file string, s Setting) bool {
	var c Config
	meta, err := toml.DecodeFile(file, &c)
	return err == nil && meta.IsDefined(append(strings.Split(s.Section, "."), s.Key)...)
}

// target is the file a change to s is written to: the one whose value takes
// effect, so the change is not overridden by an include, else the main
// config. Lists are changed in the main config, whose items come first.
func target(mainPath string, s Setting) (string, error) {
	if s.Kind == KindList {
		return mainPath, nil
	}
	source, err := Source(mainPath, s)
	if err != nil || source == "" {
		return mainPath, err
	}
	return source, nil
}

// Set saves value, as s.Value returns it, for s, and returns the file it was
// written to. Only that setting's lines change. A value "carrier config
// check" rejects is not written, nor is one that would leave the config
// unreadable.
func Set(mainPath string, s Setting, value any) (string, error) {
	file, err := target(mainPath, s)
	if err != nil {
		return "", err
	}
	old, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	base := old
	if len(strings.TrimSpace(string(base))) == 0 {
		base = []byte(NewFileHeader)
	}
	updated, err := SetValue(base, s.Section, s.Key, Literal(value))
	if err != nil {
		return "", err
	}
	return file, write(mainPath, file, updated, settingFields(s))
}

// Reset removes s from the file whose value takes effect, so the default, or
// the value in an earlier file, applies again. It returns the file changed,
// or "" when no file set s.
func Reset(mainPath string, s Setting) (string, error) {
	file, err := Source(mainPath, s)
	if err != nil || file == "" {
		return "", err
	}
	old, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return file, write(mainPath, file, RemoveValue(old, s.Section, s.Key), settingFields(s))
}

// settingFields matches the fields "carrier config check" reports s as,
// such as "redaction.patterns[2]" for an item of a list.
func settingFields(s Setting) func(string) bool {
	return func(field string) bool { return field == s.Name() || strings.HasPrefix(field, s.Name()+"[") }
}

// write checks the config as it would be with file holding content, and
// writes it when it loads and has no errors in the fields that match.
func write(mainPath, file string, content []byte, fields func(string) bool) error {
	cfg, err := load(mainPath, func(path string) ([]byte, error) {
		if path == file {
			return content, nil
		}
		return os.ReadFile(path)
	})
	if err != nil {
		return fmt.Errorf("the config would not load: %w", err)
	}
	for _, issue := range Check(cfg) {
		if issue.Level == IssueError && fields(issue.Field) {
			return errors.New(issue.Message)
		}
	}
	// Write through a symlink, as dotfile managers link the config, instead
	// of replacing the link.
	if resolved, err := filepath.EvalSymlinks(file); err == nil {
		file = resolved
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(file); err == nil {
		mode = info.Mode().Perm()
	} else if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	// Write a temporary file and rename it over the config, so a crash
	// never leaves a half-written config behind.
	tmp, err := os.CreateTemp(filepath.Dir(file), ".config-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
