package replicasql

import (
	"fmt"
	"sort"
	"strings"
)

// Capture is done by SQL triggers, not by the driver's update hooks. Reasons:
//   - A trigger runs inside the statement's own transaction, so the change record and the row
//     edit commit or roll back together, which the contract needs.
//   - The pre-update hook (modernc v1.40.0 and later) sees only one connection and calls Go code
//     per row. A trigger is pure SQL, survives a driver change, and works for the Go CLI, the
//     TUI, the MCP server and any other process that opens the file.
//   - The hook gives raw column values and rowids, so uuid and foreign key lookups would need
//     extra queries from inside the hook. A trigger does them in the same statement.
// The price is that the trigger text is generated from the registry and must be rebuilt when the
// registry or a table's columns change (Refresh does that).

const (
	selfDev  = `(SELECT device FROM replica_clock WHERE id = 1)`
	hlcNow   = `(SELECT phys || '-' || printf('%04X', cnt) || '-' || node FROM replica_clock WHERE id = 1)`
	baseSeq  = `COALESCE((SELECT seq FROM replica_vector WHERE device = ` + selfDev + `), 0)`
	schemaV  = `(SELECT schema_version FROM replica_clock WHERE id = 1)`
	seenJSON = `(SELECT json_group_object(device, seq) FROM replica_vector)`
	gate     = `(SELECT applying FROM replica_clock WHERE id = 1) = 0`
	nowStamp = `strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`
	genUUID  = `lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6)))`

	// tick moves the hybrid logical clock forward once. All the right-hand sides read the old row.
	tick = `UPDATE replica_clock SET
  cnt = CASE WHEN ` + nowStamp + ` > phys THEN 0 ELSE MIN(cnt + 1, 65535) END,
  phys = CASE WHEN ` + nowStamp + ` > phys THEN ` + nowStamp + ` ELSE phys END
WHERE id = 1;`

	// bump lifts the own sequence counter to the highest sequence this device has written.
	bump = `INSERT INTO replica_vector (device, seq)
VALUES (` + selfDev + `, COALESCE((SELECT MAX(seq) FROM replica_changes WHERE device = ` + selfDev + `), 0))
ON CONFLICT(device) DO UPDATE SET seq = MAX(seq, excluded.seq);`
)

func q(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func lit(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }

// valueExpr is the JSON text of a column as it goes on the wire. row is NEW, OLD or a table alias.
func valueExpr(c Column, row string) string {
	col := row + "." + q(c.Name)
	switch {
	case c.Poly != nil:
		kinds := make([]string, 0, len(c.Poly.Targets))
		for k := range c.Poly.Targets {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		var b strings.Builder
		fmt.Fprintf(&b, "json_quote(CASE %s.%s", row, q(c.Poly.KindColumn))
		for _, k := range kinds {
			fmt.Fprintf(&b, " WHEN %s THEN (SELECT uuid FROM %s WHERE id = %s)", lit(k), q(c.Poly.Targets[k]), col)
		}
		b.WriteString(" END)")
		return b.String()
	case c.Ref != "":
		return fmt.Sprintf("json_quote((SELECT uuid FROM %s WHERE id = %s))", q(c.Ref), col)
	}
	return "json_quote(" + col + ")"
}

func opExpr(c Column, row string) string {
	return fmt.Sprintf("CASE WHEN %s.%s IS NULL THEN 'unset' ELSE 'set' END", row, q(c.Name))
}

func fieldValue(c Column, row string) string {
	return fmt.Sprintf("CASE WHEN %s.%s IS NULL THEN NULL ELSE %s END", row, q(c.Name), valueExpr(c, row))
}

func seenFor(c Column) string {
	if c.Policy == "keep-both" {
		return seenJSON
	}
	return "NULL"
}

func (d *Described) keyNew() string {
	if d.NullUUID {
		return fmt.Sprintf("(SELECT uuid FROM %s WHERE rowid = NEW.rowid)", q(d.Name))
	}
	return "NEW.uuid"
}

const changeCols = `(collection, key, field, device, seq, hlc, op, value, schema_version, seen, status)`

// triggerSQL returns the statements that install the capture triggers of one table.
func (d *Described) triggerSQL() []string {
	t := q(d.Name)
	insert := func(branches []string, key string) string {
		return fmt.Sprintf(`INSERT OR REPLACE INTO replica_changes %s
SELECT %s, %s, f, %s, %s + ROW_NUMBER() OVER (ORDER BY ord), %s, op, val, %s, seen, 'applied'
FROM (
%s
);`, changeCols, lit(d.Name), key, selfDev, baseSeq, hlcNow, schemaV, strings.Join(branches, "\nUNION ALL\n"))
	}
	restore := fmt.Sprintf(`SELECT 0 AS ord, '' AS f, 'restore' AS op, NULL AS val, NULL AS seen
WHERE EXISTS (SELECT 1 FROM replica_changes WHERE collection = %s AND key = %s AND field = '' AND op = 'delete')`, lit(d.Name), d.keyNew())

	var ins, upd []string
	ins = append(ins, restore)
	var changed []string
	for i, c := range d.Columns {
		cond := fmt.Sprintf("NEW.%s IS NOT NULL", q(c.Name))
		if c.NotNull {
			cond = "1"
		}
		ins = append(ins, fmt.Sprintf("SELECT %d AS ord, %s AS f, %s AS op, %s AS val, %s AS seen WHERE %s", i+1, lit(c.Name), opExpr(c, "NEW"), fieldValue(c, "NEW"), seenFor(c), cond))
		chg := fmt.Sprintf("OLD.%s IS NOT NEW.%s", q(c.Name), q(c.Name))
		if c.Poly != nil {
			chg = fmt.Sprintf("(%s OR OLD.%s IS NOT NEW.%s)", chg, q(c.Poly.KindColumn), q(c.Poly.KindColumn))
		}
		changed = append(changed, chg)
		upd = append(upd, fmt.Sprintf("SELECT %d AS ord, %s AS f, %s AS op, %s AS val, %s AS seen WHERE %s", i+1, lit(c.Name), opExpr(c, "NEW"), fieldValue(c, "NEW"), seenFor(c), chg))
	}
	if len(d.Columns) == 0 {
		return nil
	}
	var out []string
	name := func(s string) string { return q("rpl_" + d.Name + "_" + s) }
	for _, s := range []string{"ai", "au", "ad"} {
		out = append(out, "DROP TRIGGER IF EXISTS "+name(s))
	}
	var fill string
	if d.NullUUID {
		fill = fmt.Sprintf("UPDATE %s SET uuid = %s WHERE rowid = NEW.rowid AND uuid IS NULL;\n", t, genUUID)
	}
	out = append(out, fmt.Sprintf("CREATE TRIGGER %s AFTER INSERT ON %s WHEN %s BEGIN\n%s%s\n%s\n%s\nEND",
		name("ai"), t, gate, fill, tick, insert(ins, d.keyNew()), bump))
	out = append(out, fmt.Sprintf("CREATE TRIGGER %s AFTER UPDATE ON %s WHEN %s AND (%s) BEGIN\n%s\n%s\n%s\nEND",
		name("au"), t, gate, strings.Join(changed, " OR "), tick, insert(upd, "NEW.uuid"), bump))
	out = append(out, fmt.Sprintf(`CREATE TRIGGER %s AFTER DELETE ON %s WHEN %s AND OLD.uuid IS NOT NULL BEGIN
%s
INSERT OR REPLACE INTO replica_changes %s
VALUES (%s, OLD.uuid, '', %s, %s + 1, %s, 'delete', NULL, %s, %s, 'applied');
%s
END`, name("ad"), t, gate, tick, changeCols, lit(d.Name), selfDev, baseSeq, hlcNow, schemaV, seenJSON, bump))
	// An UPDATE of a table whose uuid was just filled in must not look like an edit: the WHEN above
	// only fires for synced columns, and uuid is not one of them.
	return out
}

// baselineSQL returns statements that write a change for every current value of the table, for a
// database that turns sync on with data in it. Rows are not touched. Entries that exist are kept.
// The clock of a row is its updated_at (capped at now), so the oldest data loses to newer edits.
func (d *Described) baselineSQL() []string {
	t := q(d.Name)
	stamp := nowStamp
	if d.HasUpdate {
		ts := `strftime('%Y-%m-%dT%H:%M:%fZ', r.updated_at)`
		stamp = fmt.Sprintf(`CASE WHEN r.updated_at IS NOT NULL AND %s IS NOT NULL AND %s < %s THEN %s ELSE %s END`, ts, ts, nowStamp, ts, nowStamp)
	}
	hlc := fmt.Sprintf(`%s || '-0000-' || (SELECT node FROM replica_clock WHERE id = 1)`, stamp)
	var out []string
	if d.NullUUID {
		out = append(out, fmt.Sprintf("UPDATE %s SET uuid = %s WHERE uuid IS NULL", t, genUUID))
	}
	for _, c := range d.Columns {
		where := "r.uuid IS NOT NULL"
		if !c.NotNull {
			where += fmt.Sprintf(" AND r.%s IS NOT NULL", q(c.Name))
		}
		out = append(out, fmt.Sprintf(`INSERT OR IGNORE INTO replica_changes %s
SELECT %s, r.uuid, %s, %s, %s + ROW_NUMBER() OVER (ORDER BY r.rowid), %s, %s, %s, %s, %s, 'applied'
FROM %s r WHERE %s`, changeCols, lit(d.Name), lit(c.Name), selfDev, baseSeq, hlc, opExpr(c, "r"), fieldValue(c, "r"), schemaV, seenForBaseline(c), t, where))
		out = append(out, bump)
	}
	return out
}

func seenForBaseline(Column) string { return "NULL" }
