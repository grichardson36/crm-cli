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

// oldBirthdaysKey is the module key the Dates view had when it was the Birthdays view.
const oldBirthdaysKey = "birthdays"

// MigrateConfig moves settings stored under the key of the renamed Birthdays view (CRM-022: the view
// is now Dates, key "dates") to the new key. It is a one-time rewrite of config.toml, not an alias:
// after it the old key no longer exists anywhere. It moves the names in modules.order and
// modules.hidden and the key bindings under [keys] from view-birthdays to view-dates (a binding
// already written for view-dates wins). The file is written only when something moved. A missing or
// unreadable file is left alone.
func MigrateConfig(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	path := ConfigFile(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil
	}
	changed := false
	if t, ok := raw["modules"].(map[string]any); ok {
		for _, key := range []string{"order", "hidden"} {
			list, ok := t[key].([]any)
			if !ok {
				continue
			}
			var out []any
			seen := map[string]bool{}
			moved := false
			for _, e := range list {
				s, isStr := e.(string)
				if isStr && (strings.EqualFold(strings.TrimSpace(s), oldBirthdaysKey) || strings.EqualFold(strings.TrimSpace(s), "birthday")) {
					s, moved = "dates", true
					e = s
				}
				if isStr {
					if seen[s] {
						continue
					}
					seen[s] = true
				}
				out = append(out, e)
			}
			if moved {
				t[key] = out
				changed = true
			}
		}
	}
	if t, ok := raw["keys"].(map[string]any); ok {
		if v, has := t["view-"+oldBirthdaysKey]; has {
			if _, taken := t["view-dates"]; !taken {
				t["view-dates"] = v
			}
			delete(t, "view-"+oldBirthdaysKey)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`, `crm keys set`, `crm modules` and the Settings view.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
