package dav

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// DefaultAddr is where `crm dav serve` listens unless told otherwise.
const DefaultAddr = "127.0.0.1:5232"

// DefaultUser is the account name when none is given.
const DefaultUser = "crm"

// IsLoopback reports whether a listen address is reachable only from this machine.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// BookNameFor names the address book after the database file, so each domain database shows up
// as its own book: personal.db gives "CRM personal".
func BookNameFor(dbPath string) string {
	stem := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
	switch strings.ToLower(stem) {
	case "", ":memory:", "crm":
		return "CRM contacts"
	}
	return "CRM " + stem
}

// BookNameFromDB names the address book after the main database file of an open connection.
func BookNameFromDB(ctx context.Context, db *sql.DB) string {
	var file string
	if err := db.QueryRowContext(ctx, `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file); err != nil {
		return BookNameFor("")
	}
	return BookNameFor(file)
}

// Info is what a person needs to connect a device.
type Info struct {
	BaseURL        string              `json:"base_url"`
	User           string              `json:"user"`
	AddressBook    string              `json:"address_book"`
	AddressBookURL string              `json:"address_book_url"`
	AddressBookDir string              `json:"address_book_path"`
	DiscoveryURL   string              `json:"discovery_url"`
	Listen         string              `json:"listen"`
	Password       string              `json:"password"`
	Steps          map[string][]string `json:"steps"`
	Notes          []string            `json:"notes"`
}

// BuildInfo assembles the connection details and the setup steps. The password is never known
// here: it comes from the environment or a file when the server starts.
func BuildInfo(baseURL, user, bookName string) Info {
	if user == "" {
		user = DefaultUser
	}
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "http://" + DefaultAddr
	}
	book := BookPath(user)
	in := Info{
		BaseURL: base, User: user, AddressBook: bookName,
		AddressBookDir: book, AddressBookURL: base + book,
		DiscoveryURL: base + "/.well-known/carddav",
		Listen:       DefaultAddr,
		Password:     "the one you gave `crm dav serve` (--password-env or --password-file)",
	}
	in.Steps = map[string][]string{
		"android": {
			"Install DAVx5 and open it.",
			"Tap + (add account), then choose \"Login with URL and user name\".",
			fmt.Sprintf("Base URL: %s", base+"/dav/"),
			fmt.Sprintf("User name: %s. Password: the server password.", user),
			"Tap Login. DAVx5 finds the address book \"" + bookName + "\".",
			"Under CardDAV tick the address book \"" + bookName + "\" and tap the sync button. Keep \"Contacts\" permission granted.",
			"Set the sync interval you want (the CRM answers instantly, 30 minutes is typical) and leave \"Force read-only\" off to edit both ways.",
		},
		"ios": {
			"Open Settings, Contacts, Accounts, Add Account, Other, Add CardDAV Account.",
			fmt.Sprintf("Server: %s (the host name only works with the default /.well-known path). User name: %s. Password: the server password.", hostOf(base), user),
			"Tap Next. If iOS cannot discover the book, open Advanced Settings and set the account URL to " + in.AddressBookURL + ".",
		},
		"macos": {
			"Open Contacts, then Contacts menu, Add Account, Other Contacts Account, CardDAV.",
			fmt.Sprintf("Account type Manual. User name: %s. Password: the server password. Server address: %s.", user, hostOf(base)),
			"If it does not find the book, edit the account and use " + in.AddressBookURL + " as the server path.",
		},
	}
	in.Notes = []string{
		"The server speaks plain HTTP. Put a TLS-terminating reverse proxy in front before the phone connects over a network.",
		"All active people sync. Archived and merged people are removed from the phone. Deleting a contact on the phone archives it in the CRM (restore with `crm person restore`).",
		"When the phone and the CRM changed the same field, the newest change wins and the clash is logged: `crm dav conflicts`.",
	}
	return in
}

func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}
