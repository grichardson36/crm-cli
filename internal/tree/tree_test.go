package tree_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathParsing(t *testing.T) {
	tests := []struct {
		path, parent, name string
		depth              int
	}{
		{"work", "", "work", 1},
		{"work/clients", "work", "clients", 2},
		{"work/clients/acme", "work/clients", "acme", 3},
		{"", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.parent, tree.Parent(tt.path))
			assert.Equal(t, tt.name, tree.Name(tt.path))
			assert.Equal(t, tt.depth, tree.Depth(tt.path))
		})
	}
	assert.Equal(t, []string{"a", "b"}, tree.Split("a/b"))
	assert.Equal(t, "a/b", tree.Join("a", "", "b"))
}

func TestRebaseAndSubtree(t *testing.T) {
	tests := []struct {
		path, from, to, want string
		under                bool
	}{
		{"work", "work", "job", "job", false},
		{"work/clients", "work", "job", "job/clients", true},
		{"work/clients/acme", "work/clients", "x/y", "x/y/acme", true},
		{"workshop", "work", "job", "workshop", false},
		{"home", "work", "job", "home", false},
	}
	for _, tt := range tests {
		t.Run(tt.path+">"+tt.to, func(t *testing.T) {
			assert.Equal(t, tt.want, tree.Rebase(tt.path, tt.from, tt.to))
			assert.Equal(t, tt.under, tree.IsUnder(tt.path, tt.from))
		})
	}
}

func paths(nodes []tree.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Path)
	}
	return out
}

func TestBuildVirtualParentsOrderAndCounts(t *testing.T) {
	items := []tree.Item{
		{Path: "work/clients/acme", ID: 3, Own: 2, Refs: []string{"p1", "p2"}},
		{Path: "work", ID: 1, Own: 1, Refs: []string{"p1"}},
		{Path: "Home", ID: 4, Own: 1, Refs: []string{"p9"}},
		{Path: "work/clients/beta", ID: 5, Own: 1, Refs: []string{"p2"}},
	}
	nodes := tree.Build(items)
	require.Equal(t, []string{"Home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, paths(nodes))
	by := map[string]tree.Node{}
	for _, n := range nodes {
		by[n.Path] = n
	}
	assert.True(t, by["work/clients"].Virtual, "no row names work/clients")
	assert.False(t, by["work"].Virtual)
	tests := []struct {
		path                  string
		own, total, kids, des int
		depth                 int
	}{
		{"work", 1, 2, 1, 3, 1},         // p1 is counted once: it is on work and on acme
		{"work/clients", 0, 2, 2, 2, 2}, // p1 is not under clients, p2 is on both children
		{"work/clients/acme", 2, 2, 0, 0, 3},
		{"Home", 1, 1, 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			n := by[tt.path]
			assert.Equal(t, tt.own, n.Own)
			assert.Equal(t, tt.total, n.Total)
			assert.Equal(t, tt.kids, n.Children)
			assert.Equal(t, tt.des, n.Descendants)
			assert.Equal(t, tt.depth, n.Depth)
		})
	}
}

func TestBuildWithoutRefsSumsOwn(t *testing.T) {
	nodes := tree.Build([]tree.Item{{Path: "a/b", Own: 2}, {Path: "a", Own: 1}, {Path: "a/c", Own: 4}})
	require.Equal(t, []string{"a", "a/b", "a/c"}, paths(nodes))
	assert.Equal(t, 7, nodes[0].Total)
	assert.Equal(t, 1, nodes[0].Own)
}

func TestBuildSameCaseSiblingsAreDistinct(t *testing.T) {
	nodes := tree.Build([]tree.Item{{Path: "Work", Own: 1}, {Path: "work", Own: 1}})
	assert.Equal(t, []string{"Work", "work"}, paths(nodes))
}
