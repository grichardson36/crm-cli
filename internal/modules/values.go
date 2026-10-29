package modules

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Table reads one table of config.toml as a map. It is nil when the file or the table is missing or the
// file is not valid TOML. Settings that hold lists or several keys (web search engines, colors) use it.
func Table(dataDir, table string) map[string]any {
	if dataDir == "" {
		return nil
	}
	data, err := os.ReadFile(ConfigFile(dataDir))
	if err != nil {
		return nil
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil
	}
	t, _ := raw[table].(map[string]any)
	return t
}

// SetTable replaces one table of config.toml with the given keys and keeps every other table. An empty
// map removes the table. Values may be strings or lists of strings.
func SetTable(dataDir, table string, values map[string]any) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	path := ConfigFile(dataDir)
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("%s is not valid TOML (%v): fix or remove it before changing settings", path, err)
		}
	}
	if len(values) == 0 {
		delete(raw, table)
	} else {
		raw[table] = values
	}
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`, `crm keys set`, `crm modules` and the Settings view.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
