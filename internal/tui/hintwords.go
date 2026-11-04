package tui

// The words of the key hints (patterns/key-hints-in-one-place, CRM-023). Every label in the footer, a
// popup or a mode comes from this table, so a verb reads the same wherever it shows. The registry
// labels (footShort, footLabel, footDyn) and every fk(...) hint use these constants, and
// TestHintLabelsAreNeverLiterals keeps a literal out of them.
//
// Verb words, one per verb:
//
//	d      delete (purge only in the Archived view)
//	a      add
//	l      link
//	u      unlink
//	r      restore
//	f      favorite
//	s      snooze
//	m      merge
//	enter  open in a list, edit in the detail pane, then by mode: save (writes a value), pick (chooses
//	       from a list), run (a command or palette item), apply (a prepared command). Compare, accept,
//	       rename and load where enter does that instead of opening.
//	tab    details (focus the detail pane of a list)
//	space  select (marks rows), toggle (flips a value in place, the Compare screen)
//	esc    cancel (editors, prompts, pickers, confirms), back (leaves a screen, pane, detail or tag
//	       page), close (a read-only popup, help, history)
const (
	// Verbs.
	hwDelete    = "delete"
	hwPurge     = "purge"
	hwAdd       = "add"
	hwLink      = "link"
	hwUnlink    = "unlink"
	hwRestore   = "restore"
	hwFavorite  = "favorite"
	hwSnooze    = "snooze"
	hwMerge     = "merge"
	hwOpen      = "open"
	hwEdit      = "edit"
	hwDetails   = "details"
	hwSave      = "save"
	hwPick      = "pick"
	hwRun       = "run"
	hwApply     = "apply"
	hwCompare   = "compare"
	hwAccept    = "accept"
	hwRename    = "rename"
	hwLoad      = "load"
	hwSelect    = "select"
	hwToggle    = "toggle"
	hwCancel    = "cancel"
	hwBack      = "back"
	hwClose     = "close"
	hwYes       = "yes"
	hwNo        = "no"
	hwDone      = "done"
	hwContacted = "contacted"
	hwMark      = "mark"
	hwUndo      = "undo"
	hwRedo      = "redo"
	hwCopy      = "copy"
	hwReset     = "reset"
	hwAttach    = "attach"
	hwSync      = "sync"
	hwDryRun    = "dry-run"
	hwTag       = "tag"
	hwNote      = "note"
	hwTask      = "task"
	hwNew       = "new"
	hwCadence   = "cadence"

	// Chrome and navigation.
	hwFind    = "find"
	hwHelp    = "help"
	hwViews   = "views"
	hwOptions = "options"
	hwMove    = "move"

	// Screen specifics.
	hwOutside    = "outside"
	hwCell       = "cell"
	hwNextField  = "next field"
	hwSurvivor   = "survivor"
	hwPair       = "pair"
	hwPane       = "pane"
	hwLevel      = "level"
	hwMode       = "mode"
	hwChange     = "change"
	hwChoose     = "choose"
	hwComplete   = "complete"
	hwNewLine    = "new line"
	hwChangeKind = "change kind"
	hwType       = "type"
	hwVerbs      = "verbs"
	hwScroll     = "scroll"
	hwTopBottom  = "top or bottom"
	hwEveryThing = "every action"
	hwBindIt     = "bind it"
	hwPickPerson = "pick a person"
	hwCreate     = "create a contact"
	hwKeepText   = "keep as text"
	hwSameAnswer = "same answer for the name"
	hwRow        = "row"
	hwColumn     = "column"
)

// hintCanon lists the labels a hint key may carry, by key. A key that is not here has no rule.
// It is the check of TestHintLabelsStayInTheWordTable.
var hintCanon = map[string][]string{
	"d":     {hwDelete, hwPurge},
	"a":     {hwAdd},
	"l":     {hwLink},
	"u":     {hwUnlink},
	"r":     {hwRestore},
	"f":     {hwFavorite},
	"s":     {hwSnooze},
	"m":     {hwMerge},
	"esc":   {hwCancel, hwBack, hwClose},
	"space": {hwSelect, hwToggle},
	"y":     {hwYes},
	"n":     {hwNo},
}
