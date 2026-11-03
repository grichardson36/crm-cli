package query_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/stretchr/testify/assert"
)

// CRM-019, list door: the People list search and filters see the middle name. Rows keep "First Last"
// and the name sort is unchanged.
func TestMiddleNameInSearchAndFilter(t *testing.T) {
	d := fixture(t)
	exec(t, d, `UPDATE people SET middle_name = 'Marigold' WHERE first_name = 'Bob'`)
	exec(t, d, `UPDATE people SET middle_name = 'Zed' WHERE first_name = 'Ann'`)

	assert.Equal(t, []string{"Bob Brown"}, labels(run(t, d, "person", query.Query{Search: "marigold"})), "plain search finds it")
	assert.Equal(t, []string{"Bob Brown"}, labels(run(t, d, "person", query.Query{Search: "bob mari"})), "with the first name in the same search")
	assert.Equal(t, []string{"Bob Brown"}, labels(run(t, d, "person", where(cond("middle", query.OpContains, "gold")))), "filter by field")
	assert.Equal(t, []string{"Ann Alder", "Bob Brown"}, labels(run(t, d, "person", where(cond("middle", query.OpNotEmpty)))))
	assert.Equal(t, []string{"Ann Alder", "Bob Brown", "Cy Cole", "Dee Dunn", "Eve Egan", "Fay Frost"},
		labels(run(t, d, "person", query.Query{})), "the name sort ignores the middle name")
}
