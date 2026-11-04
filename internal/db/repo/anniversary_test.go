package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022: an anniversary lives on the spouse link row. Every test opens a fresh in-memory database.

func annivPerson(t *testing.T, pr *repo.PersonRepo, first, last string) int64 {
	t.Helper()
	in := model.CreatePersonInput{FirstName: first}
	if last != "" {
		in.LastName = &last
	}
	p, err := pr.Create(context.Background(), in)
	require.NoError(t, err)
	return p.ID
}

func setupAnniv(t *testing.T) (*repo.LinkRepo, *repo.PersonRepo) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return repo.NewLinkRepo(d), repo.NewPersonRepo(d)
}

func annivRef(id int64) repo.Ref { return repo.Ref{Kind: "person", ID: id} }

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// annivOf is the anniversary of the link as person a sees it.
func annivOf(t *testing.T, links *repo.LinkRepo, a, linkID int64) *string {
	t.Helper()
	items, err := links.Linked(context.Background(), annivRef(a), "person")
	require.NoError(t, err)
	for _, it := range items {
		if it.LinkID == linkID {
			return it.Anniversary
		}
	}
	t.Fatalf("link %d not listed for person %d", linkID, a)
	return nil
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

// spousePair makes two spouses with a shared last name and the given anniversary.
func spousePair(t *testing.T, links *repo.LinkRepo, pr *repo.PersonRepo, f1, f2, last, anniv string) (a, b, linkID int64) {
	t.Helper()
	a, b = annivPerson(t, pr, f1, last), annivPerson(t, pr, f2, last)
	at := model.LinkAttrs{Kind: sp("spouse")}
	if anniv != "" {
		at.Anniversary = sp(anniv)
	}
	res, err := links.Link(context.Background(), annivRef(a), annivRef(b), at)
	require.NoError(t, err)
	return a, b, res.LinkID
}

func TestAnniversariesWindow_S1_TitleYearsDate(t *testing.T) {
	links, pr := setupAnniv(t)
	joey, _, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
	got, err := pr.AnniversariesWindow(context.Background(), 90, 100, day(2026, 6, 17))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Joey-Hannah Osgood Anniversary", got[0].Title)
	assert.Equal(t, "2026-06-20", got[0].Date)
	assert.Equal(t, 3, got[0].DaysUntil)
	require.NotNil(t, got[0].Years)
	assert.Equal(t, 12, *got[0].Years)
	assert.Equal(t, joey, got[0].PersonID, "a row opens the first partner")
}

func TestAnniversariesWindow_LookBackAndWindowEdges(t *testing.T) {
	links, pr := setupAnniv(t)
	spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
	ctx := context.Background()

	got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 7, 1))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "2026-06-20", got[0].Date)
	assert.Equal(t, -11, got[0].DaysUntil, "a past anniversary has a negative distance")

	got, err = pr.AnniversariesWindow(ctx, 10, 100, day(2026, 7, 1))
	require.NoError(t, err)
	assert.Empty(t, got, "11 days back is outside a 10 day look-back")

	got, err = pr.AnniversariesWindow(ctx, 0, 3, day(2026, 6, 17))
	require.NoError(t, err)
	assert.Len(t, got, 1, "3 days ahead is inside a 3 day window")
	got, err = pr.AnniversariesWindow(ctx, 0, 2, day(2026, 6, 17))
	require.NoError(t, err)
	assert.Empty(t, got, "3 days ahead is outside a 2 day window")
}

func TestAnniversariesWindow_S2_YearlessHasNoYears(t *testing.T) {
	links, pr := setupAnniv(t)
	spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "--06-20")
	got, err := pr.AnniversariesWindow(context.Background(), 90, 100, day(2026, 6, 17))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Years)
	assert.Equal(t, "2026-06-20", got[0].Date)
}

func TestAnniversariesWindow_Feb29(t *testing.T) {
	links, pr := setupAnniv(t)
	spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2016-02-29")
	ctx := context.Background()
	got, err := pr.AnniversariesWindow(ctx, 0, 100, day(2026, 2, 1))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "2026-02-28", got[0].Date, "Feb 29 falls on Feb 28 in a non-leap year")
	got, err = pr.AnniversariesWindow(ctx, 0, 100, day(2028, 2, 1))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "2028-02-29", got[0].Date, "a leap year keeps Feb 29")
}

func TestAnniversariesWindow_S4_Titles(t *testing.T) {
	ctx := context.Background()
	t.Run("different last names", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b := annivPerson(t, pr, "Joey", "Osgood"), annivPerson(t, pr, "Hannah", "Smith")
		_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("2014-06-20")})
		require.NoError(t, err)
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Joey Osgood-Hannah Smith Anniversary", got[0].Title)
	})
	t.Run("first names in stored link order, lower id first", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b := annivPerson(t, pr, "Zed", "Kay"), annivPerson(t, pr, "Amy", "Kay")
		_, err := links.Link(ctx, annivRef(b), annivRef(a), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("2014-06-20")})
		require.NoError(t, err)
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Zed-Amy Kay Anniversary", got[0].Title, "a symmetric kind is stored once with the lower id first")
		assert.Equal(t, a, got[0].PersonID)
	})
	t.Run("a partner without a last name", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b := annivPerson(t, pr, "Cy", ""), annivPerson(t, pr, "Di", "Lee")
		_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("--06-20")})
		require.NoError(t, err)
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Cy-Di Lee Anniversary", got[0].Title)
	})
	t.Run("two partners without a last name", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b := annivPerson(t, pr, "Cy", ""), annivPerson(t, pr, "Di", "")
		_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("--06-20")})
		require.NoError(t, err)
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Cy-Di Anniversary", got[0].Title)
	})
}

func TestAnniversariesWindow_S4_Exclusions(t *testing.T) {
	ctx := context.Background()
	t.Run("an archived partner drops the row", func(t *testing.T) {
		links, pr := setupAnniv(t)
		_, b, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		require.NoError(t, pr.Archive(ctx, b))
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("an archived first partner drops the row", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, _, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		require.NoError(t, pr.Archive(ctx, a))
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("a deleted link drops the row", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		got, err := pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		require.Len(t, got, 1)
		_, err = links.Unlink(ctx, annivRef(a), annivRef(b))
		require.NoError(t, err)
		got, err = pr.AnniversariesWindow(ctx, 90, 100, day(2026, 6, 17))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("a spouse link without a date does not appear", func(t *testing.T) {
		links, pr := setupAnniv(t)
		spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "")
		got, err := pr.AnniversariesWindow(ctx, 90, 365, day(2026, 6, 17))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("a link of another kind never appears, even with a date written to its row", func(t *testing.T) {
		links, pr := setupAnniv(t)
		a, b := annivPerson(t, pr, "Ed", "Fox"), annivPerson(t, pr, "Gus", "Fox")
		res, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("parent")})
		require.NoError(t, err)
		_, err = links.DB().ExecContext(ctx, `UPDATE person_relation SET anniversary = '2000-06-20' WHERE id = ?`, res.LinkID)
		require.NoError(t, err)
		got, err := pr.AnniversariesWindow(ctx, 90, 365, day(2026, 6, 17))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestLinkAnniversary_OnlySpouseKind(t *testing.T) {
	ctx := context.Background()
	links, pr := setupAnniv(t)
	a, b := annivPerson(t, pr, "Al", "Kay"), annivPerson(t, pr, "Bo", "Kay")

	t.Run("another kind refuses it", func(t *testing.T) {
		_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("friend"), Anniversary: sp("2000-03-04")})
		require.Error(t, err)
		assert.ErrorIs(t, err, model.ErrValidation)
		assert.Contains(t, err.Error(), "spouse")
	})
	t.Run("the default kind refuses it", func(t *testing.T) {
		_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Anniversary: sp("2000-03-04")})
		assert.ErrorIs(t, err, model.ErrValidation)
	})
	t.Run("an organization link does not carry it", func(t *testing.T) {
		orgs := repo.NewOrgRepo(links.DB())
		o1, _ := orgs.Create(ctx, model.CreateOrgInput{Name: "A Co"})
		o2, _ := orgs.Create(ctx, model.CreateOrgInput{Name: "B Co"})
		_, err := links.Link(ctx, repo.Ref{Kind: "organization", ID: o1.ID}, repo.Ref{Kind: "organization", ID: o2.ID}, model.LinkAttrs{Anniversary: sp("2000-03-04")})
		assert.ErrorIs(t, err, model.ErrValidation)
	})
	t.Run("a bad date is refused", func(t *testing.T) {
		for _, bad := range []string{"soon", "2014-13-01", "2014-02-30", "06-31", "--02-30", "2014", "2014-06"} {
			_, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp(bad)})
			assert.ErrorIs(t, err, model.ErrValidation, bad)
		}
	})
	t.Run("a user kind named like a spouse does not take it", func(t *testing.T) {
		_, err := repo.NewKindRepo(links.DB()).Add(ctx, "person", "life partner", "")
		require.NoError(t, err)
		_, err = links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("life partner"), Anniversary: sp("2000-03-04")})
		assert.ErrorIs(t, err, model.ErrValidation, "the rule is the built-in kind's uuid")
	})
	t.Run("the spouse kind has the built-in uuid", func(t *testing.T) {
		k, _, err := repo.NewKindRepo(links.DB()).Resolve(ctx, "person", "spouse")
		require.NoError(t, err)
		assert.Equal(t, model.SpouseKindUUID, k.UUID)
	})
	t.Run("MM-DD is stored as --MM-DD and the link says it can carry one", func(t *testing.T) {
		res, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("06-20")})
		require.NoError(t, err)
		items, err := links.Linked(ctx, annivRef(a), "person")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, res.LinkID, items[0].LinkID)
		assert.Equal(t, "--06-20", strOf(items[0].Anniversary))
		assert.True(t, items[0].CanAnniversary)
	})
	t.Run("a link of another kind says it cannot", func(t *testing.T) {
		c := annivPerson(t, pr, "Cy", "Dee")
		_, err := links.Link(ctx, annivRef(a), annivRef(c), model.LinkAttrs{Kind: sp("friend")})
		require.NoError(t, err)
		items, err := links.Linked(ctx, annivRef(c), "person")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.False(t, items[0].CanAnniversary)
	})
}

func TestLinkAnniversary_S6_UpdateClearAndUndo(t *testing.T) {
	ctx := context.Background()
	links, pr := setupAnniv(t)
	a, b, id := spousePair(t, links, pr, "Al", "Bo", "Kay", "")

	before, err := links.Snapshot(ctx, annivRef(a), annivRef(b))
	require.NoError(t, err)
	_, err = links.Update(ctx, "person_relation", id, model.LinkAttrs{Anniversary: sp("2014-06-20")})
	require.NoError(t, err)
	assert.Equal(t, "2014-06-20", strOf(annivOf(t, links, b, id)), "both partners share one date")
	after, err := links.Snapshot(ctx, annivRef(a), annivRef(b))
	require.NoError(t, err)
	assert.False(t, repo.SameRecords(before, after))

	require.NoError(t, links.Apply(ctx, repo.UndoSet(before, after)))
	assert.Nil(t, annivOf(t, links, a, id), "undo puts the old value back")
	require.NoError(t, links.Apply(ctx, after))
	assert.Equal(t, "2014-06-20", strOf(annivOf(t, links, a, id)), "redo")

	_, err = links.Update(ctx, "person_relation", id, model.LinkAttrs{Anniversary: sp("")})
	require.NoError(t, err)
	assert.Nil(t, annivOf(t, links, a, id), "an empty string clears it")
}

func TestLinkAnniversary_KindChangeAndRelink(t *testing.T) {
	ctx := context.Background()
	links, pr := setupAnniv(t)
	a, b, id := spousePair(t, links, pr, "Al", "Bo", "Kay", "2014-06-20")

	t.Run("Update refuses it on a link of another kind", func(t *testing.T) {
		c := annivPerson(t, pr, "Cy", "Dee")
		res, err := links.Link(ctx, annivRef(a), annivRef(c), model.LinkAttrs{Kind: sp("friend")})
		require.NoError(t, err)
		_, err = links.Update(ctx, "person_relation", res.LinkID, model.LinkAttrs{Anniversary: sp("2000-01-01")})
		assert.ErrorIs(t, err, model.ErrValidation)
	})
	t.Run("relinking after an unlink brings the date back", func(t *testing.T) {
		_, err := links.Unlink(ctx, annivRef(a), annivRef(b))
		require.NoError(t, err)
		r, err := links.Link(ctx, annivRef(a), annivRef(b), model.LinkAttrs{Kind: sp("spouse")})
		require.NoError(t, err)
		assert.True(t, r.Revived)
		assert.Equal(t, "2014-06-20", strOf(annivOf(t, links, a, id)))
	})
	t.Run("a link that stops being a spouse link loses its date", func(t *testing.T) {
		_, err := links.Update(ctx, "person_relation", id, model.LinkAttrs{Kind: sp("friend")})
		require.NoError(t, err)
		rec, err := links.Get(ctx, "person_relation", id)
		require.NoError(t, err)
		assert.Nil(t, rec.Anniversary)
	})
}

func TestPersonMerge_S9_KeepsAnniversary(t *testing.T) {
	ctx := context.Background()
	t.Run("the moved link keeps its date", func(t *testing.T) {
		links, pr := setupAnniv(t)
		keep := annivPerson(t, pr, "Joe", "Osgood")
		dup, _, id := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		_, err := pr.Merge(ctx, keep, []int64{dup}, nil)
		require.NoError(t, err)
		items, err := links.Linked(ctx, annivRef(keep), "person")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, id, items[0].LinkID)
		assert.Equal(t, "2014-06-20", strOf(items[0].Anniversary))
	})
	t.Run("a duplicate link takes the dropped link's date when the survivor has none", func(t *testing.T) {
		links, pr := setupAnniv(t)
		keep := annivPerson(t, pr, "Joe", "Osgood")
		dup, hannah, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		_, err := links.Link(ctx, annivRef(keep), annivRef(hannah), model.LinkAttrs{Kind: sp("spouse")})
		require.NoError(t, err)
		_, err = pr.Merge(ctx, keep, []int64{dup}, nil)
		require.NoError(t, err)
		items, err := links.Linked(ctx, annivRef(keep), "person")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "2014-06-20", strOf(items[0].Anniversary))
	})
	t.Run("a duplicate link keeps the survivor's own date", func(t *testing.T) {
		links, pr := setupAnniv(t)
		keep := annivPerson(t, pr, "Joe", "Osgood")
		dup, hannah, _ := spousePair(t, links, pr, "Joey", "Hannah", "Osgood", "2014-06-20")
		_, err := links.Link(ctx, annivRef(keep), annivRef(hannah), model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("2015-01-02")})
		require.NoError(t, err)
		_, err = pr.Merge(ctx, keep, []int64{dup}, nil)
		require.NoError(t, err)
		items, err := links.Linked(ctx, annivRef(keep), "person")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "2015-01-02", strOf(items[0].Anniversary))
	})
}
