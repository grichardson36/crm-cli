# Anniversaries

A spouse link can carry an anniversary. The Dates view of the terminal UI lists anniversaries and birthdays together.

## Set an anniversary

An anniversary belongs to a link of the built-in `spouse` kind, so both partners share one date. Use `YYYY-MM-DD`, or `--MM-DD` when the year is unknown (use the `=` form so the shell does not read it as a flag). No other kind takes one: the attribute is refused with exit code 2.

```sh crm-example
crm person link person 1 2 --kind spouse --anniversary 2014-06-20
crm person link person 1 2 --kind spouse --anniversary=--06-20
crm person link person 1 2 --kind spouse --anniversary ""
crm person relate 1 2 --type spouse --anniversary 2014-06-20
```

An empty value clears it. `crm person show` lists it on the link as `anniversary`. MCP: `anniversary` on `crm_person_link`, listed by `crm_links`. A person merge keeps the date and its undo restores it. A link that stops being a spouse link loses its date.

`crm birthdays` and the MCP tool `crm_birthdays` list birthdays only.

## In the terminal UI

Picking Spouse in the Link form asks one more line, "Anniversary (blank skips)". A date that is not valid is refused with a message and asked again. On the person page the date is a cell of the link row: `right` moves to it, `enter` edits it and `ctrl+z` undoes it.

The Dates view (key 7) groups by month, then by day with a divider. On one day the birthdays come first, then the anniversaries. An anniversary row reads "Joey-Hannah Osgood Anniversary", or "Joey Osgood-Hannah Smith Anniversary" when the last names differ, with the years at the right edge (blank when the year is unknown). Enter opens the first partner. Archived people and deleted links are left out. A Feb 29 date falls on Feb 28 in a year without one, like a birthday.

`ctrl+o` has one row, Show: birthdays and anniversaries (the default), birthdays only, or anniversaries only.

The view was called Birthdays before (module key `birthdays`). A stored arrangement or key binding under that name moves to `dates` when `config.toml` is first read. The old name is not accepted.

See also: birthdays, links, relations, modules, tui.
