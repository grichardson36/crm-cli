package theme

import (
	"fmt"
	"strings"
)

// Mode says whether the screen is light or dark. auto follows the terminal. light and dark are the
// person's choice: the themes that follow the terminal take that variant, and the screen is painted
// with the theme background so the whole screen matches whatever the terminal itself shows.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeLight Mode = "light"
	ModeDark  Mode = "dark"
)

// Modes lists the modes in the order the Appearance row shows them.
var Modes = []Mode{ModeAuto, ModeLight, ModeDark}

// ModeKey is the key in config.toml, next to `theme`.
const ModeKey = "mode"

// ParseMode reads a setting value ("" is auto).
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return ModeAuto, nil
	case ModeAuto, ModeLight, ModeDark:
		return m, nil
	}
	return ModeAuto, fmt.Errorf("mode %q is not valid, use auto, light or dark", s)
}

// ChosenMode is the remembered mode of this device. A missing or invalid value is auto.
func ChosenMode(dataDir string) Mode {
	m, err := ParseMode(readConfig(dataDir)[ModeKey])
	if err != nil {
		return ModeAuto
	}
	return m
}

// SaveMode remembers the mode for this device. Other settings in the file are kept.
func SaveMode(dataDir string, m Mode) error {
	if _, err := ParseMode(string(m)); err != nil {
		return err
	}
	return saveKey(dataDir, ModeKey, string(m))
}

// Dark reports whether the themes that follow the terminal take their dark variant.
// terminalDark is what the terminal reported about its own background.
func (m Mode) Dark(terminalDark bool) bool {
	switch m {
	case ModeLight:
		return false
	case ModeDark:
		return true
	}
	return terminalDark
}

// Fill is how the screen is painted for this mode: auto paints only when the theme and the terminal
// disagree, light and dark always paint.
func (m Mode) Fill() Fill {
	if m == ModeLight || m == ModeDark {
		return FillOn
	}
	return FillAuto
}
