package icsio

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
)

// volatile lines change on every write without the event changing.
var volatile = []string{"DTSTAMP", "CREATED", "LAST-MODIFIED", "X-WR-CALNAME"}

func lines(data []byte) []string {
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
}

// Canonical strips the lines that change on every write, so two exports of the same event
// compare equal.
func Canonical(data []byte) string {
	var out []string
	for _, line := range lines(data) {
		skip := false
		for _, v := range volatile {
			if strings.HasPrefix(line, v+":") || strings.HasPrefix(line, v+";") {
				skip = true
			}
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// EventICS exports one event (with its overrides) the way it would be sent to a calendar server,
// and returns the UID it carries.
func EventICS(ctx context.Context, db *sql.DB, source string, eventID int64) (data []byte, uid string, err error) {
	res, err := Export(ctx, db, ExportOptions{EventIDs: []int64{eventID}, Source: source})
	if err != nil {
		return nil, "", err
	}
	for _, line := range lines(res.Data) {
		if strings.HasPrefix(line, "UID:") {
			uid = strings.TrimPrefix(line, "UID:")
			break
		}
	}
	return res.Data, uid, nil
}

// Fingerprint is a hash of everything an export of the event carries, ignoring write times.
func Fingerprint(ctx context.Context, db *sql.DB, source string, eventID int64) (string, error) {
	data, _, err := EventICS(ctx, db, source, eventID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(Canonical(data)))
	return hex.EncodeToString(sum[:]), nil
}
