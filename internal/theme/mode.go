package theme

import (
	"fmt"
	"strings"
)

// Mode says whether the screen is light or dark. auto follows the terminal and never paints the
// background. light and dark are the person's choice: every theme takes that palette, and the screen
// is painted with its background so the whole screen matches whatever the terminal itself shows.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeLight Mode = "light"
	ModeDark  Mode = "dark"
)

// Modes lists the modes in the order the Theme pane shows them.
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

// Dark reports whether the dark palette is used. terminalDark is what the terminal reported about its
// own background, auto follows it.
func (m Mode) Dark(terminalDark bool) bool {
	switch m {
	case ModeLight:
		return false
	case ModeDark:
		return true
	}
	return terminalDark
}

// Paints reports whether the screen is painted with the palette background. auto never paints, light
// and dark always do.
func (m Mode) Paints() bool { return m == ModeLight || m == ModeDark }
