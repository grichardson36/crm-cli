package repo

import (
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
)

// The built-in spouse kind is the only kind that carries an anniversary (CRM-022). It is found by
// uuid, never by name. The uuid is the one migration 008 gives the row, or the one turning on device
// sync gives every relation_kind row (a uuid derived from object type and name, so all devices agree),
// so both count.
func spouseKindUUIDs() [2]string {
	return [2]string{model.SpouseKindUUID, replicasql.NaturalUUID("relation_kind", "person", "spouse")}
}

// IsSpouseKind reports whether the kind is the built-in spouse kind.
func IsSpouseKind(k *model.RelationKind) bool {
	if k == nil {
		return false
	}
	u := spouseKindUUIDs()
	return k.UUID == u[0] || k.UUID == u[1]
}

// spouseKindSQL is an SQL condition that is true for the spouse kind row aliased k. The uuids come
// from the function above, never from input.
func spouseKindSQL() string {
	u := spouseKindUUIDs()
	return "k.uuid IN ('" + u[0] + "', '" + u[1] + "')"
}
