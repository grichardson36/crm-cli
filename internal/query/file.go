package query

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// FileVersion is the version of the TOML file a saved view exports to.
const FileVersion = 1

// File is a saved view as a small TOML file for sharing (patterns/saved-view-query). It names the kind,
// filter, sort and group and nothing personal: no ids, no uuids, no dates of creation. A relation value
// is the name of the thing, which the importing database looks up.
//
//	version = 1
//	name = "Overdue follow-up"
//	kind = "person"
//
//	[[filter]]
//	field = "last_contacted"
//	op = "before"
//	values = ["90 days ago"]
type File struct {
	Version int    `toml:"version"`
	Name    string `toml:"name"`
	Kind    string `toml:"kind"`
	Query
}

// Encode writes the file.
func (f File) Encode() (string, error) {
	f.Version = FileVersion
	var b bytes.Buffer
	b.WriteString("# crm saved view. Import it with: crm views import FILE\n")
	if err := toml.NewEncoder(&b).Encode(f); err != nil {
		return "", fmt.Errorf("write view file: %w", err)
	}
	return b.String(), nil
}

// DecodeFile reads a view file. An unknown key, a wrong version or a file that is not TOML is a
// validation error: nothing is guessed.
func DecodeFile(data []byte) (File, error) {
	var f File
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return File{}, validate.Fail(validate.Issue{Code: validate.CodeQueryValue, Field: "file", Expected: "a saved view TOML file", Received: err.Error(), Fix: "export a view with crm views export to see the format"})
	}
	if un := md.Undecoded(); len(un) > 0 {
		keys := make([]string, len(un))
		for i, k := range un {
			keys[i] = k.String()
		}
		return File{}, validate.Fail(validate.Issue{Code: validate.CodeQueryField, Field: "file", Expected: "only the keys of a saved view file", Received: strings.Join(keys, ", "), Fix: "remove the unknown keys"})
	}
	if f.Version != FileVersion {
		return File{}, validate.Fail(validate.Issue{Code: validate.CodeQueryValue, Field: "version", Expected: fmt.Sprintf("version = %d", FileVersion), Received: fmt.Sprint(f.Version), Fix: "this build reads version 1 files"})
	}
	return f, nil
}
