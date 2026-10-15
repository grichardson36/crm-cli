package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every color lives in this package. Views take styles from the active theme, so a theme change
// reaches all of them. This test fails when a color literal or a direct color call appears
// anywhere else in the program.
var hardcoded = []*regexp.Regexp{
	regexp.MustCompile(`lipgloss\.(Color|AdaptiveColor|CompleteColor|CompleteAdaptiveColor)\b`),
	regexp.MustCompile(`\.(Foreground|Background|BorderForeground|BorderBackground)\(`),
	regexp.MustCompile(`"#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?"`),
	regexp.MustCompile(`termenv\.(RGBColor|ANSIColor|ANSI256Color|ForegroundColor|BackgroundColor)`),
	regexp.MustCompile(`\\x1b\[|\\033\[|\\u001b\[`),
}

func TestNoHardcodedColorsOutsideThemePackage(t *testing.T) {
	root := filepath.Join("..", "..")
	var scanned int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".claude", "theme", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			line = strings.ReplaceAll(line, "context.Background(", "context.Bg(") // not a color
			for _, re := range hardcoded {
				if re.MatchString(line) {
					t.Errorf("%s:%d hardcoded color (%s): move it into internal/theme: %s", path, i+1, re, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, scanned, 50, "the scan must see the program's source files")
}
