package repo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/phone"
)

// PhoneNormalizeReport is the outcome of `crm phone normalize`. Counts are of person_phones rows.
type PhoneNormalizeReport struct {
	Region string `json:"region"`
	// Total is every stored phone number.
	Total int `json:"total"`
	// Fill is rows whose E.164 form is missing or differs from the parsed one. Applied is true when
	// they were written.
	Fill int `json:"fill"`
	// Current is rows that already hold the right E.164 form.
	Current int `json:"current"`
	// Unparseable is rows that cannot be read as a possible number. They keep no E.164 form and match
	// the old way (last 10 digits).
	Unparseable int `json:"unparseable"`
	// Unlikely is rows that parse but do not exist in their region's numbering plan (they still get an
	// E.164 form, the same as the W_PHONE_UNLIKELY warning).
	Unlikely int `json:"unlikely"`
	// Duplicates is people who hold the same number twice in different formats. They are reported, not
	// merged: the rows keep their typed text.
	Duplicates int  `json:"duplicates"`
	Applied    bool `json:"applied"`
	// Examples lists the first changes, for the dry run.
	Examples []PhoneChange `json:"examples,omitempty"`
}

// PhoneChange is one phone row the normalization fills.
type PhoneChange struct {
	PersonID int64  `json:"person_id"`
	Typed    string `json:"typed"`
	Old      string `json:"old,omitempty"`
	E164     string `json:"e164"`
	Valid    bool   `json:"valid"`
}

// Line is the one-line summary.
func (r PhoneNormalizeReport) Line() string {
	verb := "would fill"
	if r.Applied {
		verb = "filled"
	}
	return fmt.Sprintf("%d phone numbers (region %s): %s %d, %d already current, %d cannot be parsed, %d look invalid for the region, %d duplicate numbers on one person",
		r.Total, r.Region, verb, r.Fill, r.Current, r.Unparseable, r.Unlikely, r.Duplicates)
}

type phoneRow struct {
	id       int64
	personID int64
	typed    string
	e164     sql.NullString
}

// NormalizePhones fills person_phones.e164 for every stored number from the typed text and the
// default region. The typed text is never changed. Without apply nothing is written. With apply
// every change is made in one transaction, so it is all or nothing, and running it again changes
// nothing. A row that cannot be parsed has its e164 cleared, so the column always matches the typed
// text and the current region.
func (r *PersonRepo) NormalizePhones(ctx context.Context, apply bool, examples int) (*PhoneNormalizeReport, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, person_id, phone_number, e164 FROM person_phones ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list phones: %w", err)
	}
	var all []phoneRow
	for rows.Next() {
		var pr phoneRow
		if err := rows.Scan(&pr.id, &pr.personID, &pr.typed, &pr.e164); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan phone: %w", err)
		}
		all = append(all, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()

	rep := &PhoneNormalizeReport{Region: phone.DefaultRegion(), Total: len(all)}
	type update struct {
		id   int64
		e164 any
	}
	var updates []update
	seen := map[[2]string]bool{}
	for _, pr := range all {
		n := phone.Parse(pr.typed)
		if n.Parsed && !n.Valid {
			rep.Unlikely++
		}
		if n.E164 == "" {
			rep.Unparseable++
		} else {
			k := [2]string{fmt.Sprint(pr.personID), n.Key()}
			if seen[k] {
				rep.Duplicates++
			}
			seen[k] = true
		}
		if pr.e164.String == n.E164 {
			rep.Current++
			continue
		}
		rep.Fill++
		if len(rep.Examples) < examples {
			rep.Examples = append(rep.Examples, PhoneChange{PersonID: pr.personID, Typed: pr.typed, Old: pr.e164.String, E164: n.E164, Valid: n.Valid})
		}
		updates = append(updates, update{pr.id, nullIfEmpty(n.E164)})
	}
	if !apply || len(updates) == 0 {
		return rep, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE person_phones SET e164 = ? WHERE id = ?`, u.e164, u.id); err != nil {
			return nil, fmt.Errorf("fill e164: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rep.Applied = true
	return rep, nil
}

// ClearPhoneE164 empties person_phones.e164 on every row, the undo of NormalizePhones. The typed
// text was never changed, so nothing is lost: matching and display read the typed text again. It
// returns the number of rows cleared.
func (r *PersonRepo) ClearPhoneE164(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE person_phones SET e164 = NULL WHERE e164 IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("clear e164: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// FillPhoneE164 fills the missing E.164 forms of one person's numbers. Importers and merges insert
// rows directly and call it afterwards.
func FillPhoneE164(ctx context.Context, q DBTX, personID int64) error {
	rows, err := q.QueryContext(ctx, `SELECT id, phone_number FROM person_phones WHERE person_id = ? AND e164 IS NULL`, personID)
	if err != nil {
		return fmt.Errorf("list phones: %w", err)
	}
	type fill struct {
		id   int64
		e164 string
	}
	var fills []fill
	for rows.Next() {
		var id int64
		var typed string
		if err := rows.Scan(&id, &typed); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan phone: %w", err)
		}
		if e := phone.E164(typed); e != "" {
			fills = append(fills, fill{id, e})
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, f := range fills {
		if _, err := q.ExecContext(ctx, `UPDATE person_phones SET e164 = ? WHERE id = ?`, f.e164, f.id); err != nil {
			return fmt.Errorf("fill e164: %w", err)
		}
	}
	return nil
}
