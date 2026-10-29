package dav_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appleCard is a vCard as an iPhone writes it: PRODID of the device, grouped itemN labels, several
// TEL lines with mixed TYPE case, a partial birthday, unknown X- properties and a folded NOTE.
const appleCard = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"PRODID:-//Apple Inc.//iPhone OS 17.5//EN\n" +
	"N:Smith;Jane;;;\n" +
	"FN:Jane Smith\n" +
	"ORG:Acme;\n" +
	"TITLE:Engineer\n" +
	"item1.TEL;type=pref:+1 (415) 555-2671\n" +
	"item1.X-ABLabel:_$!<Mobile>!$_\n" +
	"TEL;type=WORK;type=VOICE:(415) 555-0199\n" +
	"item2.TEL:+44 20 7946 0958\n" +
	"item2.X-ABLabel:Mum's flat\n" +
	"item3.EMAIL;type=INTERNET:jane@example.com\n" +
	"item3.X-ABLabel:work thing\n" +
	"NOTE:A long note that the phone folded onto a second line because it\n" +
	"  was far too long for one line of a vCard\n" +
	"BDAY;X-APPLE-OMIT-YEAR=1604:1604-03-14\n" +
	"X-ABADR:us\n" +
	"X-ABUID:5C1B5A0E-1111-2222-3333-444455556666:ABPerson\n" +
	"X-CUSTOM-FLAG;X-PARAM=odd:keep me\n" +
	"UID:apple-uid-1\n" +
	"REV:2026-10-01T10:00:00Z\n" +
	"END:VCARD\n"

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

// putCard sends a card the way a phone does and returns the response.
func (e *env) putCard(davID, card string, hdr map[string]string) *http.Response {
	e.t.Helper()
	h := map[string]string{"Content-Type": vcardHdr}
	for k, v := range hdr {
		h[k] = v
	}
	return e.do("PUT", bookPath+davID+".vcf", crlf(card), h, true)
}

// getCard fetches the card text and its ETag.
func (e *env) getCard(davID string) (string, string) {
	e.t.Helper()
	resp := e.do("GET", bookPath+davID+".vcf", "", nil, true)
	require.Equal(e.t, http.StatusOK, resp.StatusCode)
	return bodyOf(e.t, resp), strings.Trim(resp.Header.Get("ETag"), `"`)
}

func (e *env) personOf(davID string) *model.Person {
	e.t.Helper()
	tg, err := e.davRep.Resolve(context.Background(), davID)
	require.NoError(e.t, err)
	return e.reload(tg.PersonID)
}

// lines splits card text into lines for a line-by-line comparison.
func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\r\n"), "\r\n") }

// diff returns the lines only in a and only in b.
func diff(a, b string) (onlyA, onlyB []string) {
	inA, inB := map[string]bool{}, map[string]bool{}
	for _, l := range lines(a) {
		inA[l] = true
	}
	for _, l := range lines(b) {
		inB[l] = true
	}
	for _, l := range lines(a) {
		if !inB[l] {
			onlyA = append(onlyA, l)
		}
	}
	for _, l := range lines(b) {
		if !inA[l] {
			onlyB = append(onlyB, l)
		}
	}
	return
}

func TestAppleCardRoundTripIsByteForByte(t *testing.T) {
	e := newEnv(t, false)
	resp := e.putCard("apple-uid-1", appleCard, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	p := e.personOf("apple-uid-1")

	t.Run("the CRM understands the structure", func(t *testing.T) {
		assert.Equal(t, "Jane", p.FirstName)
		assert.Equal(t, "Engineer", *p.Title)
		require.Len(t, p.Phones, 3)
		labels := map[string]string{}
		for _, ph := range p.Phones {
			labels[ph.E164] = *ph.Label
		}
		assert.Equal(t, "Mobile", labels["+14155552671"])
		assert.Equal(t, "Work", labels["+14155550199"])
		assert.Equal(t, "Mum's flat", labels["+442079460958"])
		require.NotNil(t, p.BirthdayMonth)
		assert.Nil(t, p.BirthdayYear, "year 1604 means no year")
	})

	t.Run("an unchanged contact comes back exactly as the phone sent it", func(t *testing.T) {
		got, _ := e.getCard("apple-uid-1")
		assert.Equal(t, crlf(appleCard), got)
		again, _ := e.getCard("apple-uid-1")
		assert.Equal(t, got, again)
	})

	t.Run("the same card sent again changes nothing and keeps the phone copy", func(t *testing.T) {
		before := e.personOf("apple-uid-1").UpdatedAt
		_, etag := e.getCard("apple-uid-1")
		e.clock = e.clock.Add(0)
		resp := e.putCard("apple-uid-1", appleCard, map[string]string{"If-Match": etagHeader(etag)})
		assert.Less(t, resp.StatusCode, 300)
		assert.NotEmpty(t, strings.Trim(resp.Header.Get("ETag"), `"`), "the server copy is the phone copy")
		assert.Equal(t, before, e.personOf("apple-uid-1").UpdatedAt)
		assert.Empty(t, e.conflicts())
		got, _ := e.getCard("apple-uid-1")
		assert.Equal(t, crlf(appleCard), got)
	})

	t.Run("a CRM-only change touches only its own line", func(t *testing.T) {
		_, err := e.people.Update(context.Background(), p.ID, model.UpdatePersonInput{Title: ptr("Staff Engineer")})
		require.NoError(t, err)
		e.bump(p.ID)
		got, _ := e.getCard("apple-uid-1")
		onlyOld, onlyNew := diff(crlf(appleCard), got)
		assert.Equal(t, []string{"TITLE:Engineer"}, onlyOld)
		assert.Equal(t, []string{"TITLE:Staff Engineer"}, onlyNew)
		assert.Contains(t, got, "PRODID:-//Apple Inc.//iPhone OS 17.5//EN")
		assert.Contains(t, got, "REV:2026-10-01T10:00:00Z", "REV is the phone's, never touched")
		assert.Contains(t, got, "UID:apple-uid-1")
		assert.Contains(t, got, "X-CUSTOM-FLAG;X-PARAM=odd:keep me")
		assert.Contains(t, got, "item2.X-ABLabel:Mum's flat")
		assert.Contains(t, got, "NOTE:A long note that the phone folded onto a second line because it\r\n  was far too long")
	})

	t.Run("a REPORT returns the same text", func(t *testing.T) {
		body := `<?xml version="1.0"?><C:addressbook-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav"><D:prop><D:getetag/><C:address-data/></D:prop><D:href>` +
			bookPath + "apple-uid-1.vcf</D:href></C:addressbook-multiget>"
		resp := e.do("REPORT", bookPath, body, map[string]string{"Depth": "1", "Content-Type": "application/xml"}, true)
		require.Equal(t, http.StatusMultiStatus, resp.StatusCode)
		var ms struct {
			Data string `xml:"response>propstat>prop>address-data"`
		}
		require.NoError(t, xml.NewDecoder(resp.Body).Decode(&ms))
		want, _ := e.getCard("apple-uid-1")
		assert.Equal(t, want, ms.Data)
		assert.Contains(t, ms.Data, "  was far too long", "folding survives the report")
	})
}

func TestOutboundListEditsKeepNeighbours(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", appleCard, nil).StatusCode)
	p := e.personOf("apple-uid-1")
	e.getCard("apple-uid-1") // the phone holds the card

	t.Run("a number added in the CRM is appended", func(t *testing.T) {
		require.NoError(t, e.people.AddPhone(ctx, p.ID, "(212) 555-0147", ptr("Home"), false))
		e.bump(p.ID)
		got, _ := e.getCard("apple-uid-1")
		onlyOld, onlyNew := diff(crlf(appleCard), got)
		assert.Empty(t, onlyOld)
		require.Len(t, onlyNew, 1)
		assert.Contains(t, onlyNew[0], "TEL;TYPE=HOME:(212) 555-0147")
	})

	t.Run("a number removed in the CRM takes its label group with it", func(t *testing.T) {
		require.NoError(t, e.people.RemovePhone(ctx, p.ID, "+44 20 7946 0958"))
		e.bump(p.ID)
		got, _ := e.getCard("apple-uid-1")
		assert.NotContains(t, got, "item2.TEL")
		assert.NotContains(t, got, "Mum's flat")
		assert.Contains(t, got, "item1.TEL;type=pref:+1 (415) 555-2671", "other numbers are untouched")
		assert.Contains(t, got, "item1.X-ABLabel:_$!<Mobile>!$_")
		assert.Contains(t, got, "item3.X-ABLabel:work thing")
	})

	t.Run("the same number in another format is not a change", func(t *testing.T) {
		before, _ := e.getCard("apple-uid-1")
		require.NoError(t, e.people.AddPhone(ctx, p.ID, "415.555.2671", nil, false))
		e.bump(p.ID)
		after, _ := e.getCard("apple-uid-1")
		assert.Equal(t, before, after)
		assert.Len(t, e.personOf("apple-uid-1").Phones, 3)
	})
}

func TestPartialAndFullBirthdayPatch(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", appleCard, nil).StatusCode)
	p := e.personOf("apple-uid-1")
	e.getCard("apple-uid-1")
	year := "1985-03-14"
	_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: &year})
	require.NoError(t, err)
	e.bump(p.ID)
	got, _ := e.getCard("apple-uid-1")
	onlyOld, onlyNew := diff(crlf(appleCard), got)
	assert.Equal(t, []string{"BDAY;X-APPLE-OMIT-YEAR=1604:1604-03-14"}, onlyOld)
	require.Len(t, onlyNew, 1)
	assert.Contains(t, onlyNew[0], "1985-03-14")
}

func TestInboundDeleteNeedsThePreviousCard(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", appleCard, nil).StatusCode)
	p := e.personOf("apple-uid-1")
	_, etag := e.getCard("apple-uid-1")

	// The CRM adds a number the phone has never seen. The phone then sends its edit of another
	// field with its own copy, which lacks that number. Absence is not a delete.
	require.NoError(t, e.people.AddPhone(ctx, p.ID, "(212) 555-0147", nil, false))
	e.bump(p.ID)
	edited := strings.Replace(appleCard, "TITLE:Engineer", "TITLE:Director", 1)
	require.Less(t, e.putCard("apple-uid-1", edited, map[string]string{"If-Match": etagHeader(etag)}).StatusCode, 300)
	got := e.personOf("apple-uid-1")
	assert.Equal(t, "Director", *got.Title)
	var nums []string
	for _, ph := range got.Phones {
		nums = append(nums, ph.E164)
	}
	assert.ElementsMatch(t, []string{"+14155552671", "+14155550199", "+442079460958", "+12125550147"}, nums)

	// A number the phone held and then removes is a delete.
	fetched, etag := e.getCard("apple-uid-1")
	noWork := strings.Replace(fetched, "TEL;type=WORK;type=VOICE:(415) 555-0199"+crlfEnd, "", 1)
	require.NotEqual(t, fetched, noWork)
	require.Less(t, e.putCard("apple-uid-1", noWork, map[string]string{"If-Match": etagHeader(etag)}).StatusCode, 300)
	nums = nil
	for _, ph := range e.personOf("apple-uid-1").Phones {
		nums = append(nums, ph.E164)
	}
	assert.NotContains(t, nums, "+14155550199")
	assert.Contains(t, nums, "+12125550147")
}

func TestUnreadableCardIsSkippedAndFlagged(t *testing.T) {
	e := newEnv(t, false)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", appleCard, nil).StatusCode)
	_, etag := e.getCard("apple-uid-1")
	before := e.personOf("apple-uid-1")

	for name, body := range map[string]string{
		"not a vcard":          "this is not a vcard at all",
		"truncated":            "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;;;\n",
		"line without a colon": "BEGIN:VCARD\nVERSION:3.0\nFN Jane\nEND:VCARD\n",
		"empty":                "",
	} {
		t.Run(name, func(t *testing.T) {
			resp := e.putCard("apple-uid-1", body, map[string]string{"If-Match": etagHeader(etag)})
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			after := e.personOf("apple-uid-1")
			assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "nothing was emptied")
			assert.Len(t, after.Phones, 3)
			assert.Equal(t, "Engineer", *after.Title)
		})
	}
	issues, err := e.davRep.Issues(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, issues, 4)
	assert.Equal(t, model.DavIssueUnreadable, issues[0].Kind)
	assert.Equal(t, "apple-uid-1", issues[0].DavID)
	require.NotNil(t, issues[0].PersonID)
	st, err := e.davRep.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 4, st.OpenIssues)
	assert.NoError(t, e.davRep.ResolveIssue(context.Background(), issues[0].ID))
}

func TestConflictsAreDecidedPerField(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", appleCard, nil).StatusCode)
	p := e.personOf("apple-uid-1")
	_, etag := e.getCard("apple-uid-1")

	// Both sides change the title. The CRM also changes the company, the phone also the notes.
	_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("Manager"), Location: ptr("Berlin")})
	require.NoError(t, err)
	e.bump(p.ID)
	edited := strings.Replace(appleCard, "TITLE:Engineer", "TITLE:Director", 1)
	edited = strings.Replace(edited, "FN:Jane Smith", "FN:Jane Smith\nNICKNAME:JJ", 1)
	edited = strings.Replace(edited, "item3.EMAIL;type=INTERNET:jane@example.com", "item3.EMAIL;type=INTERNET:jane@example.com\nEMAIL:second@example.com", 1)
	resp := e.putCard("apple-uid-1", edited, map[string]string{"If-Match": etagHeader(etag)})
	require.Less(t, resp.StatusCode, 300)
	assert.Empty(t, resp.Header.Get("ETag"), "the phone must fetch the merged card")

	got := e.personOf("apple-uid-1")
	assert.Equal(t, "Manager", *got.Title, "the CRM value is kept")
	assert.Equal(t, "Berlin", *got.Location, "a field only the CRM changed is untouched")
	assert.Len(t, got.Emails, 2, "a field only the phone changed is taken")
	cs := e.conflicts()
	require.Len(t, cs, 1, "only the title clashed")
	assert.Equal(t, "title", cs[0].Field)
	assert.Equal(t, "crm", cs[0].Winner)
	assert.Equal(t, "Director", *cs[0].DeviceValue)

	merged, _ := e.getCard("apple-uid-1")
	assert.Contains(t, merged, "TITLE:Manager")
	assert.Contains(t, merged, "NICKNAME:JJ", "the phone's unknown lines survive")
	assert.Contains(t, merged, "EMAIL:second@example.com")
	assert.Contains(t, merged, "PRODID:-//Apple Inc.//iPhone OS 17.5//EN")
}

func TestTitleWaterfall(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false)
	orgs := repo.NewOrgRepo(e.db)
	links := repo.NewLinkRepo(e.db)
	card := strings.Replace(appleCard, "TITLE:Engineer\n", "", 1)
	card = strings.Replace(card, "ORG:Acme;\n", "ORG:Initech;\n", 1)
	require.Equal(t, http.StatusCreated, e.putCard("apple-uid-1", card, nil).StatusCode)
	p := e.personOf("apple-uid-1")
	org, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	role, yes := "Head of Sales", true
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: p.ID}, repo.Ref{Kind: "organization", ID: org.ID}, model.LinkAttrs{Primary: &yes, Role: &role})
	require.NoError(t, err)
	e.bump(p.ID)

	t.Run("no headline: the role goes out as TITLE", func(t *testing.T) {
		got, _ := e.getCard("apple-uid-1")
		assert.Contains(t, got, "TITLE:Head of Sales")
	})
	t.Run("the headline title wins over the role", func(t *testing.T) {
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("Founder")})
		require.NoError(t, err)
		e.bump(p.ID)
		got, etag := e.getCard("apple-uid-1")
		assert.Contains(t, got, "TITLE:Founder")
		assert.NotContains(t, got, "Head of Sales")

		t.Run("an inbound edit sets the headline", func(t *testing.T) {
			edited := strings.Replace(got, "TITLE:Founder", "TITLE:CEO", 1)
			require.Less(t, e.putCard("apple-uid-1", edited, map[string]string{"If-Match": etagHeader(etag)}).StatusCode, 300)
			assert.Equal(t, "CEO", *e.personOf("apple-uid-1").Title)
			_, etag = e.getCard("apple-uid-1")

			t.Run("blanking TITLE clears the headline and the role shows again", func(t *testing.T) {
				blank := strings.Replace(edited, "TITLE:CEO\r\n", "", 1)
				require.NotEqual(t, edited, blank)
				require.Less(t, e.putCard("apple-uid-1", blank, map[string]string{"If-Match": etagHeader(etag)}).StatusCode, 300)
				now := e.personOf("apple-uid-1")
				assert.Empty(t, nonEmpty(now.Title), "the headline is cleared")
				memberships, err := links.All(ctx, repo.Ref{Kind: "person", ID: p.ID})
				require.NoError(t, err)
				assert.NotEmpty(t, memberships, "no membership was deleted")
				e.bump(p.ID)
				back, _ := e.getCard("apple-uid-1")
				assert.Contains(t, back, "TITLE:Head of Sales")
			})
		})
	})
}

func nonEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const crlfEnd = "\r\n"
