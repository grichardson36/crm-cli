package dedupe

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// nameSuffixes are dropped from the end of a last name.
var nameSuffixes = map[string]bool{
	"jr": true, "sr": true, "ii": true, "iii": true, "iv": true, "v": true,
	"md": true, "phd": true, "esq": true, "dds": true, "dvm": true, "cpa": true,
	"mba": true, "rn": true, "pe": true,
}

// namePrefixes are dropped from the start of a first name.
var namePrefixes = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "miss": true, "dr": true, "prof": true,
}

// nicknames maps a nickname or spelling variant to its canonical given name.
var nicknames = map[string]string{
	"bob": "robert", "bobby": "robert", "rob": "robert", "robbie": "robert", "robby": "robert", "bert": "robert",
	"bill": "william", "billy": "william", "will": "william", "willy": "william", "willie": "william",
	"liz": "elizabeth", "lizzie": "elizabeth", "beth": "elizabeth", "betty": "elizabeth", "eliza": "elizabeth", "libby": "elizabeth", "betsy": "elizabeth",
	"jim": "james", "jimmy": "james", "jamie": "james", "jimbo": "james",
	"jack": "john", "johnny": "john", "johnnie": "john",
	"jon":  "jonathan",
	"rick": "richard", "ricky": "richard", "rich": "richard", "richie": "richard", "dick": "richard",
	"mike": "michael", "mikey": "michael", "mick": "michael",
	"tom": "thomas", "tommy": "thomas",
	"charlie": "charles", "chuck": "charles", "chas": "charles",
	"joe": "joseph", "joey": "joseph",
	"dan": "daniel", "danny": "daniel",
	"matt":  "matthew",
	"chris": "christopher", "topher": "christopher",
	"tony": "anthony",
	"andy": "andrew", "drew": "andrew",
	"ed": "edward", "eddie": "edward", "ted": "edward", "teddy": "edward", "ned": "edward",
	"maggie": "margaret", "meg": "margaret", "peggy": "margaret", "marge": "margaret",
	"kate": "katherine", "katie": "katherine", "kathy": "katherine", "kat": "katherine", "katy": "katherine",
	"catherine": "katherine", "kathryn": "katherine", "katharine": "katherine", "cathy": "katherine",
	"pat": "patricia", "patty": "patricia", "trish": "patricia", "tricia": "patricia",
	"jen": "jennifer", "jenny": "jennifer",
	"jess": "jessica", "jessie": "jessica",
	"sue": "susan", "susie": "susan", "suzy": "susan",
	"deb": "deborah", "debbie": "deborah",
	"barb": "barbara", "barbie": "barbara",
	"nick": "nicholas", "nicky": "nicholas",
	"sam": "samuel", "sammy": "samuel",
	"ben": "benjamin", "benny": "benjamin", "benji": "benjamin",
	"alex":  "alexander",
	"steve": "stephen", "stevie": "stephen", "steven": "stephen",
	"tim": "timothy", "timmy": "timothy",
	"greg":  "gregory",
	"becky": "rebecca", "becca": "rebecca",
	"vicky": "victoria", "vicki": "victoria", "tori": "victoria",
	"fred": "frederick", "freddie": "frederick", "freddy": "frederick",
	"ken": "kenneth", "kenny": "kenneth",
	"ron": "ronald", "ronnie": "ronald",
	"don": "donald", "donnie": "donald",
	"larry": "lawrence", "laurence": "lawrence",
	"zach": "zachary", "zack": "zachary", "zak": "zachary",
	"nate": "nathan", "nathaniel": "nathan",
	"pete": "peter",
	"phil": "philip", "phillip": "philip",
	"jeff": "jeffrey", "jeffery": "jeffrey",
	"vince": "vincent", "vinny": "vincent",
	"hank": "henry", "harry": "henry",
	"abby": "abigail", "abbie": "abigail",
	"mandy":  "amanda",
	"cindy":  "cynthia",
	"pam":    "pamela",
	"steph":  "stephanie",
	"jackie": "jacqueline",
	"kim":    "kimberly",
	"josh":   "joshua",
	"dave":   "david", "davy": "david",
	"doug":  "douglas",
	"jake":  "jacob",
	"nikki": "nicole",
	"gene":  "eugene",
	"jerry": "gerald", "gerry": "gerald",
	"len": "leonard", "lenny": "leonard", "leo": "leonard",
	"walt": "walter", "wally": "walter",
}

// Canonical returns the canonical given name for a normalized first name.
func Canonical(first string) string {
	if c, ok := nicknames[first]; ok {
		return c
	}
	return first
}

// fold lowercases s, strips accents and replaces every non letter/digit rune with a space.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// accent mark, dropped
		case r == '\'' || r == '’':
			// "O'Brien" -> "obrien"
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// NormalizeName returns the first and last name used for comparison.
// Case, accents, punctuation, honorific prefixes, middle names and suffixes are ignored.
// The first token of the first name and the last token of the last name are kept.
// When the last name is empty and the first name has several tokens, the first name field
// is treated as a full name.
//
// A name that contains "@" is an email address stored as a name (common in imports). It has no
// comparable name, so both parts come back empty.
func NormalizeName(first, last string) (string, string) {
	if strings.Contains(first, "@") || strings.Contains(last, "@") {
		return "", ""
	}
	f := strings.Fields(fold(first))
	for len(f) > 0 && namePrefixes[f[0]] {
		f = f[1:]
	}
	l := strings.Fields(fold(last))
	for len(l) > 1 && nameSuffixes[l[len(l)-1]] {
		l = l[:len(l)-1]
	}
	if len(f) == 0 {
		// The first name field held only an honorific ("Dr." + "Robert Borer"): use the last name field.
		for len(l) > 0 && namePrefixes[l[0]] {
			l = l[1:]
		}
		if len(l) > 1 {
			f, l = l[:1], l[1:]
		} else {
			f, l = l, nil
		}
	}
	if len(l) == 0 && len(f) > 1 {
		for len(f) > 2 && nameSuffixes[f[len(f)-1]] {
			f = f[:len(f)-1]
		}
		l = f[len(f)-1:]
	}
	var nf, nl string
	if len(f) > 0 {
		nf = f[0]
	}
	if len(l) > 0 {
		nl = l[len(l)-1]
	}
	return nf, nl
}

// FullName joins a normalized first and last name.
func FullName(first, last string) string {
	return strings.TrimSpace(first + " " + last)
}

// PhoneKey returns the last 10 digits of a phone number, or "" when fewer than 7 digits are present.
func PhoneKey(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) < 7 {
		return ""
	}
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	return d
}

// EmailKey lowercases and trims an email address.
func EmailKey(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(e, "@") {
		return ""
	}
	return e
}

// HandleKey normalizes a social profile value: lowercase, no scheme, host, query or trailing slash.
func HandleKey(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	v = strings.TrimPrefix(v, "www.")
	if i := strings.IndexAny(v, "?#"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "/"); i >= 0 && strings.Contains(v[:i], ".") {
		v = v[i+1:]
	}
	v = strings.Trim(v, "/")
	v = strings.TrimPrefix(v, "in/")
	v = strings.TrimPrefix(v, "@")
	return v
}
