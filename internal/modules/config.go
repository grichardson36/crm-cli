package modules

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// SetValue writes one string setting under [table] in config.toml and keeps every other setting. An
// empty value removes the key, and the table when it ends up empty. The Settings view uses it for
// device settings that are plain text, such as [open] search_url.
func SetValue(dataDir, table, key, value string) error {
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
	t, _ := raw[table].(map[string]any)
	if t == nil {
		t = map[string]any{}
	}
	if value == "" {
		delete(t, key)
	} else {
		t[key] = value
	}
	if len(t) == 0 {
		delete(raw, table)
	} else {
		raw[table] = t
	}
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`, `crm keys set`, `crm modules` and the Settings view.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Value reads one string setting under [table] in config.toml. It is "" when the file, the table or the
// key is missing or the file is not valid TOML. The Tags view reads its per-device layout with it.
func Value(dataDir, table, key string) string {
	if dataDir == "" {
		return ""
	}
	data, err := os.ReadFile(ConfigFile(dataDir))
	if err != nil {
		return ""
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return ""
	}
	t, _ := raw[table].(map[string]any)
	v, _ := t[key].(string)
	return v
}
