package repo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// links moves every link of the dropped people to the survivor (Q27). Exact duplicates are dropped
// (soft deleted), memberships at the same organization fold into the richest one with the earliest
// start, and an alias records each old id and uuid so old references keep resolving.
func (m *merger) links() error {
	for _, d := range m.dropIDs {
		if err := m.moveActivities(d); err != nil {
			return err
		}
		if err := m.moveDealPeople(d); err != nil {
			return err
		}
		if err := m.movePersonRelations(d); err != nil {
			return err
		}
		if err := m.moveMemberships(d); err != nil {
			return err
		}
		if err := m.writeAlias(d); err != nil {
			return err
		}
	}
	return nil
}

func (m *merger) softDelete(table string, id int64) error {
	_, err := m.exec("UPDATE "+table+" SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ?", id)
	return err
}

func (m *merger) duplicate(n int) {
	m.res.Moved["duplicates_dropped"] += n
}

// moveActivities re-points tasks and interactions that mention a dropped person.
func (m *merger) moveActivities(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, source_kind, source_id FROM activity_target
		WHERE target_kind = 'person' AND target_id = ? AND deleted_at IS NULL`, d)
	if err != nil {
		return fmt.Errorf("list activity links: %w", err)
	}
	type row struct {
		id, src int64
		kind    string
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.kind, &r.src); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		var n int
		if err := m.tx.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM activity_target WHERE source_kind = ? AND source_id = ?
			AND target_kind = 'person' AND target_id = ? AND deleted_at IS NULL`, r.kind, r.src, m.keepID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if err := m.softDelete("activity_target", r.id); err != nil {
				return fmt.Errorf("drop duplicate activity link: %w", err)
			}
			m.duplicate(1)
			continue
		}
		if _, err := m.exec("UPDATE activity_target SET target_id = ?, updated_at = datetime('now') WHERE id = ?", m.keepID, r.id); err != nil {
			return fmt.Errorf("move activity link: %w", err)
		}
		table := "tasks"
		key := "tasks"
		if r.kind == "interaction" {
			table, key = "interactions", "interactions"
		}
		if _, err := m.exec("UPDATE "+table+" SET updated_at = datetime('now') WHERE id = ?", r.src); err != nil {
			return fmt.Errorf("touch %s: %w", table, err)
		}
		m.res.Moved[key]++
	}
	return nil
}

// moveDealPeople re-points deal links. When the survivor is already on the deal the richer role is kept.
func (m *merger) moveDealPeople(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, deal_id, role FROM deal_person WHERE person_id = ? AND deleted_at IS NULL`, d)
	if err != nil {
		return fmt.Errorf("list deal links: %w", err)
	}
	type row struct {
		id, deal int64
		role     sql.NullString
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.deal, &r.role); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		var keepID int64
		var keepRole sql.NullString
		err := m.tx.QueryRowContext(m.ctx, `SELECT id, role FROM deal_person WHERE deal_id = ? AND person_id = ? AND deleted_at IS NULL`, r.deal, m.keepID).Scan(&keepID, &keepRole)
		switch {
		case err == nil:
			if (!keepRole.Valid || keepRole.String == "") && r.role.Valid && r.role.String != "" {
				if _, err := m.exec("UPDATE deal_person SET role = ?, updated_at = datetime('now') WHERE id = ?", r.role.String, keepID); err != nil {
					return err
				}
			}
			if err := m.softDelete("deal_person", r.id); err != nil {
				return err
			}
			m.duplicate(1)
		case err == sql.ErrNoRows:
			if _, err := m.exec("UPDATE deal_person SET person_id = ?, updated_at = datetime('now') WHERE id = ?", m.keepID, r.id); err != nil {
				return fmt.Errorf("move deal link: %w", err)
			}
			if _, err := m.exec("UPDATE deals SET updated_at = datetime('now') WHERE id = ?", r.deal); err != nil {
				return err
			}
			m.res.Moved["deals"]++
		default:
			return err
		}
	}
	return nil
}

// movePersonRelations re-points typed person links. Links that become a self link or a duplicate are dropped.
func (m *merger) movePersonRelations(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT l.id, l.a_id, l.b_id, l.kind_id, k.symmetric FROM person_relation l
		JOIN relation_kind k ON k.id = l.kind_id WHERE (l.a_id = ? OR l.b_id = ?) AND l.deleted_at IS NULL`, d, d)
	if err != nil {
		return fmt.Errorf("list person links: %w", err)
	}
	type row struct {
		id, a, b, kind int64
		sym            bool
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.a, &r.b, &r.kind, &r.sym); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		a, b := r.a, r.b
		if a == d {
			a = m.keepID
		}
		if b == d {
			b = m.keepID
		}
		if a == b {
			if err := m.softDelete("person_relation", r.id); err != nil {
				return err
			}
			m.duplicate(1)
			continue
		}
		if r.sym && a > b {
			a, b = b, a
		}
		var n int
		if err := m.tx.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM person_relation WHERE a_id = ? AND b_id = ? AND kind_id = ?
			AND deleted_at IS NULL AND id <> ?`, a, b, r.kind, r.id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if err := m.softDelete("person_relation", r.id); err != nil {
				return err
			}
			m.duplicate(1)
			continue
		}
		if _, err := m.exec("UPDATE person_relation SET a_id = ?, b_id = ?, updated_at = datetime('now') WHERE id = ?", a, b, r.id); err != nil {
			return fmt.Errorf("move person link: %w", err)
		}
		m.res.Moved["relationships"]++
	}
	return nil
}

type membershipRow struct {
	id, org                int64
	role, from, to, notes  sql.NullString
	primary                bool
}

func (r membershipRow) richness() int {
	n := 0
	for _, s := range []sql.NullString{r.role, r.from, r.to, r.notes} {
		if s.Valid && s.String != "" {
			n++
		}
	}
	return n
}

func (m *merger) memberships(person int64) ([]membershipRow, error) {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, org_id, role, started_on, ended_on, notes, is_primary FROM membership
		WHERE person_id = ? AND deleted_at IS NULL ORDER BY id`, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []membershipRow
	for rows.Next() {
		var r membershipRow
		var p int
		if err := rows.Scan(&r.id, &r.org, &r.role, &r.from, &r.to, &r.notes, &p); err != nil {
			return nil, err
		}
		r.primary = p == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// moveMemberships folds a dropped person's memberships into the survivor's. A membership at an
// organization the survivor is also at, with the same end date (both open or equal), is the same
// membership: the richest row stays, with the earliest start. Others move over. A different end
// date means a different stay (left and came back) so both rows stay.
func (m *merger) moveMemberships(d int64) error {
	dropped, err := m.memberships(d)
	if err != nil {
		return fmt.Errorf("list memberships: %w", err)
	}
	for _, r := range dropped {
		keep, err := m.memberships(m.keepID)
		if err != nil {
			return err
		}
		var same *membershipRow
		for i := range keep {
			k := keep[i]
			if k.org == r.org && k.to.Valid == r.to.Valid && k.to.String == r.to.String {
				same = &keep[i]
				break
			}
		}
		if same == nil {
			hasPrimary := false
			for _, k := range keep {
				hasPrimary = hasPrimary || k.primary
			}
			primary := r.primary && !hasPrimary
			if _, err := m.exec("UPDATE membership SET person_id = ?, is_primary = ?, updated_at = datetime('now') WHERE id = ?", m.keepID, primary, r.id); err != nil {
				return fmt.Errorf("move membership: %w", err)
			}
			m.res.Moved["memberships"]++
			continue
		}
		// Fold r into same. Delete r first so a primary flag it holds is free to move.
		if err := m.softDelete("membership", r.id); err != nil {
			return err
		}
		rich := *same
		poor := r
		if r.richness() > same.richness() {
			rich, poor = r, *same
		}
		fill := func(a, b sql.NullString) sql.NullString {
			if a.Valid && a.String != "" {
				return a
			}
			return b
		}
		role, notes, to := fill(rich.role, poor.role), fill(rich.notes, poor.notes), fill(rich.to, poor.to)
		from := fill(rich.from, poor.from)
		if poor.from.Valid && poor.from.String != "" && rich.from.Valid && rich.from.String != "" && poor.from.String < rich.from.String {
			from = poor.from
		}
		primary := same.primary || r.primary
		if _, err := m.exec(`UPDATE membership SET role = ?, started_on = ?, ended_on = ?, notes = ?, is_primary = ?, updated_at = datetime('now') WHERE id = ?`,
			nullStr(role), nullStr(from), nullStr(to), nullStr(notes), primary, same.id); err != nil {
			return fmt.Errorf("merge membership: %w", err)
		}
		m.duplicate(1)
	}
	return nil
}

func nullStr(s sql.NullString) any {
	if s.Valid && s.String != "" {
		return s.String
	}
	return nil
}

// writeAlias records that the dropped person's id and uuid now resolve to the survivor. Aliases that
// already pointed at the dropped person now point at the survivor.
func (m *merger) writeAlias(d int64) error {
	var oldUUID string
	if err := m.tx.QueryRowContext(m.ctx, "SELECT uuid FROM people WHERE id = ?", d).Scan(&oldUUID); err != nil {
		return err
	}
	if _, err := m.exec(`UPDATE merge_alias SET survivor_id = ?, updated_at = datetime('now')
		WHERE kind = 'person' AND survivor_id = ? AND deleted_at IS NULL`, m.keepID, d); err != nil {
		return fmt.Errorf("repoint aliases: %w", err)
	}
	if _, err := m.exec(`INSERT INTO merge_alias (uuid, kind, old_id, old_uuid, survivor_id) VALUES (?, 'person', ?, ?, ?)
		ON CONFLICT(kind, old_id) WHERE deleted_at IS NULL DO UPDATE SET survivor_id = excluded.survivor_id, updated_at = datetime('now')`,
		uuid.New().String(), d, oldUUID, m.keepID); err != nil {
		return fmt.Errorf("write merge alias: %w", err)
	}
	m.res.Moved["aliases"]++
	return nil
}

// ResolveAlias returns the survivor a merged object's old id points at, and false when there is no alias.
func ResolveAlias(ctx context.Context, q DBTX, kind string, oldID int64) (int64, bool) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT survivor_id FROM merge_alias WHERE kind = ? AND old_id = ? AND deleted_at IS NULL", kind, oldID).Scan(&id)
	return id, err == nil
}
