// Package validate is the one validation framework (patterns/one-validation-framework). It checks,
// sanitizes and suggests corrections for every value that enters the CRM, driven by the field types
// in model.Fields. The CLI, MCP, TUI, importers, DAV and device sync all call it, and each decides
// only how to show the result. A rule is added here once.
package validate

// Stable error codes. The strings are part of the interface: the CLI prints them, MCP returns them
// and scripts match them, so they never change meaning. Add new ones to Codes below.
const (
	CodeFieldRequired    = "E_FIELD_REQUIRED"
	CodeFieldTooLong     = "E_FIELD_TOO_LONG"
	CodeFieldUnknown     = "E_FIELD_UNKNOWN"
	CodeEmailInvalid     = "E_EMAIL_INVALID"
	CodeURLInvalid       = "E_URL_INVALID"
	CodePhoneInvalid     = "E_PHONE_INVALID"
	CodeDomainInvalid    = "E_DOMAIN_INVALID"
	CodeDateInvalid      = "E_DATE_INVALID"
	CodeDateOrder        = "E_DATE_ORDER"
	CodeChoiceInvalid    = "E_CHOICE_INVALID"
	CodeNumberInvalid    = "E_NUMBER_INVALID"
	CodeNumberTooLow     = "E_NUMBER_TOO_LOW"
	CodePrimaryMultiple  = "E_PRIMARY_MEMBERSHIP_MULTIPLE"
	CodeOrgParentCycle   = "E_ORG_PARENT_CYCLE"
	CodeVocabUnknown     = "E_VOCAB_UNKNOWN"
	CodeVocabDuplicate   = "E_VOCAB_DUPLICATE"
	CodeVocabInUse       = "E_VOCAB_IN_USE"
	CodeVocabInvalid     = "E_VOCAB_INVALID"
	CodeStageNoOpen      = "E_STAGE_NO_OPEN"
	CodeStageNoWon       = "E_STAGE_NO_WON"
	CodeStageNoLost      = "E_STAGE_NO_LOST"
	CodeStageOutcome     = "E_STAGE_OUTCOME_INVALID"
	CodePathEmptySegment = "E_PATH_EMPTY_SEGMENT"
	CodePathEdgeSlash    = "E_PATH_EDGE_SLASH"
	CodePathTooDeep      = "E_PATH_TOO_DEEP"
	CodePathTooLong      = "E_PATH_TOO_LONG"
	CodePathCycle        = "E_PATH_CYCLE"
	CodeEncodingDamaged  = "W_ENCODING_DAMAGED"
	CodePhoneInTextField = "S_PHONE_IN_TEXT_FIELD"
	CodeEmailInTextField = "S_EMAIL_IN_TEXT_FIELD"
	CodeURLInTextField   = "S_URL_IN_TEXT_FIELD"
	CodeVocabCreate      = "S_VOCAB_CREATE"
)

// Kinds of code. E_ codes fail the write, W_ codes warn and S_ codes suggest. Only errors fail.
const (
	KindError      = "error"
	KindWarning    = "warning"
	KindSuggestion = "suggestion"
)

// CodeInfo describes one code for the help topic and the docs.
type CodeInfo struct {
	Code    string
	Kind    string
	Meaning string
}

// Codes is the whole vocabulary, in one place.
var Codes = []CodeInfo{
	{CodeFieldRequired, KindError, "a required field is empty"},
	{CodeFieldTooLong, KindError, "a value is longer than the field allows"},
	{CodeFieldUnknown, KindError, "the object has no such field"},
	{CodeEmailInvalid, KindError, "the text is not shaped like an email address"},
	{CodeURLInvalid, KindError, "the text is not shaped like a web address"},
	{CodePhoneInvalid, KindError, "the text is not shaped like a phone number"},
	{CodeDomainInvalid, KindError, "the text is not a domain such as example.com"},
	{CodeDateInvalid, KindError, "the text is not a date the date parser understands"},
	{CodeDateOrder, KindError, "an end date is before its start date"},
	{CodeChoiceInvalid, KindError, "the value is not one of the field's options"},
	{CodeNumberInvalid, KindError, "the text is not a number"},
	{CodeNumberTooLow, KindError, "a number is below the field's minimum"},
	{CodePrimaryMultiple, KindError, "a person would have more than one primary membership"},
	{CodeOrgParentCycle, KindError, "an organization would become its own ancestor"},
	{CodeVocabUnknown, KindError, "the value is not in the list the user manages (a deal role or deal stage that is not on the list), create it first or pass --create"},
	{CodeVocabDuplicate, KindError, "a value with that name is already on the list the user manages"},
	{CodeVocabInUse, KindError, "a value on the list the user manages is still used by records, name a replacement or keep it as plain text (a deal stage always needs a replacement)"},
	{CodeVocabInvalid, KindError, "a value for the list the user manages is empty, too long or has a line break or a pipe"},
	{CodeStageNoOpen, KindError, "the change would leave no open deal stage, at least one stage must stay open"},
	{CodeStageNoWon, KindError, "the change would leave no won deal stage, at least one stage must have the outcome won"},
	{CodeStageNoLost, KindError, "the change would leave no lost deal stage, at least one stage must have the outcome lost"},
	{CodeStageOutcome, KindError, "a deal stage outcome is not open, won or lost"},
	{CodePathEmptySegment, KindError, "a hierarchy path (a nested tag such as work/clients) has an empty segment, as in work//clients"},
	{CodePathEdgeSlash, KindError, "a hierarchy path starts or ends with /"},
	{CodePathTooDeep, KindError, "a hierarchy path has more segments than the maximum depth"},
	{CodePathTooLong, KindError, "a hierarchy path is longer than the maximum length"},
	{CodePathCycle, KindError, "the change would move a hierarchy path under itself"},
	{CodeEncodingDamaged, KindWarning, "the text holds U+FFFD, the mark of a damaged encoding"},
	{CodePhoneInTextField, KindSuggestion, "phone-like text sits in a field that is not a phone"},
	{CodeEmailInTextField, KindSuggestion, "email-like text sits in a field that is not an email"},
	{CodeURLInTextField, KindSuggestion, "web-address-like text sits in a field that is not a web address"},
	{CodeVocabCreate, KindSuggestion, "the value is not on the list the user manages yet, it can be created from the typed text"},
}

// KindOf returns the kind of a code ("" when unknown).
func KindOf(code string) string {
	for _, c := range Codes {
		if c.Code == code {
			return c.Kind
		}
	}
	return ""
}
