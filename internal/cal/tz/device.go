package tz

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Device returns the zone of this machine. Go reports the device zone only as "Local", which is
// not a name another machine can load, so this looks for the IANA name in this order:
//
//  1. the CRM_TZ environment variable
//  2. the TZ environment variable
//  3. Windows: `tzutil /g`, mapped to an IANA name
//  4. other systems: the /etc/localtime link or /etc/timezone
//
// When no name is found it returns time.Local, whose stored name is "Local".
func Device() *time.Location {
	return deviceFrom(os.Getenv, deviceNameFromSystem)
}

func deviceFrom(getenv func(string) string, system func() string) *time.Location {
	for _, name := range []string{getenv("CRM_TZ"), getenv("TZ"), system()} {
		name = strings.TrimPrefix(strings.TrimSpace(name), ":")
		if name == "" {
			continue
		}
		if loc, err := time.LoadLocation(name); err == nil && name != "Local" {
			return loc
		}
	}
	return time.Local
}

func deviceNameFromSystem() string {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tzutil", "/g").Output()
		if err != nil {
			return ""
		}
		return WindowsToIANA(strings.TrimSpace(string(out)))
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return filepath.ToSlash(target[i+len("zoneinfo/"):])
		}
	}
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// WindowsToIANA maps a Windows time zone name to an IANA name. It covers the common zones and
// returns "" for the rest.
func WindowsToIANA(name string) string {
	return windowsZones[name]
}

var windowsZones = map[string]string{
	"UTC":                            "UTC",
	"Hawaiian Standard Time":         "Pacific/Honolulu",
	"Alaskan Standard Time":          "America/Anchorage",
	"Pacific Standard Time":          "America/Los_Angeles",
	"US Mountain Standard Time":      "America/Phoenix",
	"Mountain Standard Time":         "America/Denver",
	"Central Standard Time":          "America/Chicago",
	"Canada Central Standard Time":   "America/Regina",
	"Eastern Standard Time":          "America/New_York",
	"US Eastern Standard Time":       "America/Indianapolis",
	"Atlantic Standard Time":         "America/Halifax",
	"Newfoundland Standard Time":     "America/St_Johns",
	"Central America Standard Time":  "America/Guatemala",
	"Mexico Standard Time":           "America/Mexico_City",
	"SA Pacific Standard Time":       "America/Bogota",
	"E. South America Standard Time": "America/Sao_Paulo",
	"Argentina Standard Time":        "America/Buenos_Aires",
	"GMT Standard Time":              "Europe/London",
	"Greenwich Standard Time":        "Atlantic/Reykjavik",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Romance Standard Time":          "Europe/Paris",
	"Central European Standard Time": "Europe/Warsaw",
	"GTB Standard Time":              "Europe/Bucharest",
	"FLE Standard Time":              "Europe/Kiev",
	"E. Europe Standard Time":        "Europe/Chisinau",
	"Turkey Standard Time":           "Europe/Istanbul",
	"Russian Standard Time":          "Europe/Moscow",
	"South Africa Standard Time":     "Africa/Johannesburg",
	"Egypt Standard Time":            "Africa/Cairo",
	"Israel Standard Time":           "Asia/Jerusalem",
	"Arab Standard Time":             "Asia/Riyadh",
	"Arabian Standard Time":          "Asia/Dubai",
	"India Standard Time":            "Asia/Calcutta",
	"Pakistan Standard Time":         "Asia/Karachi",
	"SE Asia Standard Time":          "Asia/Bangkok",
	"China Standard Time":            "Asia/Shanghai",
	"Singapore Standard Time":        "Asia/Singapore",
	"Taipei Standard Time":           "Asia/Taipei",
	"Tokyo Standard Time":            "Asia/Tokyo",
	"Korea Standard Time":            "Asia/Seoul",
	"W. Australia Standard Time":     "Australia/Perth",
	"Cen. Australia Standard Time":   "Australia/Adelaide",
	"AUS Central Standard Time":      "Australia/Darwin",
	"E. Australia Standard Time":     "Australia/Brisbane",
	"AUS Eastern Standard Time":      "Australia/Sydney",
	"New Zealand Standard Time":      "Pacific/Auckland",
}
