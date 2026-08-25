// Package config loads and persists errata's YAML configuration
// (~/.config/errata/config.yaml, XDG-aware) and resolves data paths.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const appName = "errata"

// Config is the user-facing configuration.
type Config struct {
	// IgnoreCommands lists command basenames that must never be recorded.
	IgnoreCommands []string
	// IgnoreDirs lists directory prefixes; errors raised under them are not recorded.
	IgnoreDirs []string
	// HintEnabled controls the gray hit hint after a captured error.
	HintEnabled bool
	// ArchiveAfterDays is the number of days after which a pending
	// (unresolved) error is archived. 0 or negative disables archiving.
	ArchiveAfterDays int
	// SuccessWindowMinutes is how long after a failure a successful
	// command in the same directory counts as "probably fixed it"
	// (dev-guide §7.2). 0 or negative disables the success prompt.
	SuccessWindowMinutes int
}

// DefaultArchiveAfterDays is the pending-error archival horizon (dev-guide
// §7.5: archive after N days, never delete).
const DefaultArchiveAfterDays = 30

// DefaultSuccessWindowMinutes is the DETECTED_SUCCESS window (dev-guide
// §7.2: 5 minutes).
const DefaultSuccessWindowMinutes = 5

// ConfigDir returns the config directory, honoring XDG_CONFIG_HOME.
// os.UserConfigDir ignores XDG on darwin (it would return ~/Library/
// Application Support) — errata is a dotfiles-style tool and the docs
// promise ~/.config, so resolve XDG/HOME by hand on every platform.
func ConfigDir() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", appName), nil
}

// DataDir returns the data directory, honoring XDG_DATA_HOME.
func DataDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", appName), nil
}

// DBPath returns the path of the SQLite database.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "errata.db"), nil
}

func configFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func newViper() (*viper.Viper, string, error) {
	p, err := configFile()
	if err != nil {
		return nil, "", err
	}
	v := viper.New()
	v.SetConfigFile(p)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		var pn *fs.PathError
		if !errors.As(err, &nf) && !errors.As(err, &pn) {
			return nil, "", fmt.Errorf("read config: %w", err)
		}
	}
	return v, p, nil
}

// Load reads the configuration. A missing config file yields defaults.
func Load() (*Config, error) {
	v, _, err := newViper()
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		IgnoreCommands:       v.GetStringSlice("ignore.commands"),
		IgnoreDirs:           v.GetStringSlice("ignore.dirs"),
		HintEnabled:          true,
		ArchiveAfterDays:     DefaultArchiveAfterDays,
		SuccessWindowMinutes: DefaultSuccessWindowMinutes,
	}
	if v.IsSet("hint.enabled") {
		cfg.HintEnabled = v.GetBool("hint.enabled")
	}
	if v.IsSet("archive_after_days") {
		cfg.ArchiveAfterDays = v.GetInt("archive_after_days")
	}
	if v.IsSet("success_window_minutes") {
		cfg.SuccessWindowMinutes = v.GetInt("success_window_minutes")
	}
	return cfg, nil
}

// IsIgnored reports whether a command run from dir falls under the
// ignore blacklist (by command basename or directory prefix).
func (c *Config) IsIgnored(command, dir string) bool {
	base := filepath.Base(command)
	for _, ig := range c.IgnoreCommands {
		if base == ig {
			return true
		}
	}
	for _, prefix := range c.IgnoreDirs {
		p := expandHome(prefix)
		if p == "" {
			continue
		}
		if dir == p || strings.HasPrefix(dir, p+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// IgnoreKind selects which blacklist an entry belongs to.
type IgnoreKind string

const (
	// IgnoreCommand matches command basenames.
	IgnoreCommand IgnoreKind = "command"
	// IgnoreDir matches working-directory prefixes.
	IgnoreDir IgnoreKind = "dir"
)

func keyFor(kind IgnoreKind) string {
	if kind == IgnoreDir {
		return "ignore.dirs"
	}
	return "ignore.commands"
}

// AddIgnore appends value to the given blacklist and persists the config.
func AddIgnore(kind IgnoreKind, value string) error {
	v, p, err := newViper()
	if err != nil {
		return err
	}
	key := keyFor(kind)
	list := v.GetStringSlice(key)
	for _, e := range list {
		if e == value {
			return nil // already present
		}
	}
	v.Set(key, append(list, value))
	return writeConfig(v, p)
}

// RemoveIgnore deletes value from the given blacklist. It reports whether
// the entry existed.
func RemoveIgnore(kind IgnoreKind, value string) (bool, error) {
	v, p, err := newViper()
	if err != nil {
		return false, err
	}
	key := keyFor(kind)
	list := v.GetStringSlice(key)
	out := list[:0]
	found := false
	for _, e := range list {
		if e == value {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return false, nil
	}
	v.Set(key, out)
	return true, writeConfig(v, p)
}

func writeConfig(v *viper.Viper, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func expandHome(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
