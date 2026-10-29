package phone

import (
	"os"

	"github.com/BurntSushi/toml"
)

// ConfigRegion reads the default region from the [phone] table of a config.toml file:
//
//	[phone]
//	region = "GB"
//
// It returns "" when the file, the table or the key is missing, or the file is not valid TOML. The
// caller passes the result to SetDefaultRegion, which ignores an unknown code.
func ConfigRegion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Phone struct {
			Region string `toml:"region"`
		} `toml:"phone"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return ""
	}
	return cfg.Phone.Region
}
