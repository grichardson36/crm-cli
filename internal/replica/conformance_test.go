package replica_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/replicatest"
)

func TestMemConformance(t *testing.T) {
	schema := replica.NewSchema(1, replica.Collection{Name: "people", Fields: []replica.Field{
		{Name: "first_name"}, {Name: "title"},
		{Name: "notes", Policy: replica.KeepBoth},
		{Name: "last_contacted_at", Policy: replica.Max},
	}})
	n := 0
	replicatest.Run(t, replicatest.Config{
		New: func(t *testing.T) replica.Adapter {
			n++
			return replica.NewMem(fmt.Sprintf("aaaa%04d", n), schema, nil)
		},
		State: func(t *testing.T, a replica.Adapter) string {
			m := a.(*replica.Mem)
			var lines []string
			for k, row := range m.Rows() {
				var fs []string
				for f, v := range row {
					fs = append(fs, f+"="+v)
				}
				sort.Strings(fs)
				lines = append(lines, k+" "+strings.Join(fs, ","))
			}
			sort.Strings(lines)
			var rv []string
			for _, r := range m.Reviews() {
				rv = append(rv, r.Key)
			}
			return strings.Join(lines, "\n") + "\nreviews:" + strings.Join(rv, ",")
		},
		Collection: "people",
		Required:   map[string]string{"first_name": `"Ann"`},
		LWW:        "title", Notes: "notes", Max: "last_contacted_at",
	})
}
