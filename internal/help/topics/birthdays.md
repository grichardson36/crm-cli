# Birthdays

Store a birthday on a person and list the upcoming ones. A spouse link can also carry an anniversary, and the Dates view of the terminal UI lists both.

## Set a birthday

Use `YYYY-MM-DD`, or `--MM-DD` when the year is unknown. Use the `=` form for the second so the shell does not read it as a flag.

```sh crm-example
crm person add "Linus Torvalds" --birthday 1969-12-28
crm person edit 1 --birthday 1985-03-14
crm person edit 2 --birthday=--07-04
crm person edit 1 --birthday ""
```

An empty value clears the birthday.

## List upcoming birthdays

```sh crm-example
crm birthdays
crm birthdays --days 90
crm birthdays -f json
```

Fields: `person_id`, `name`, `date`, `days_until`, `relative`, `turning`. `turning` is absent when the year is unknown. The default window is 30 days.

`relative` reads the distance in calendar units: `today`, `in 6d`, `in 1w 1d`, `in 2w`, `in 1mo`, `in 1mo 1w 2d`, `in 1y 2mo`, and for the past `3d ago`, `1w 2d ago`. Days roll up into weeks at 7, months are real calendar months, zero parts are left out and at most three parts show.

`crm birthdays` and the MCP tool `crm_birthdays` list birthdays only. Anniversaries are not in them.

## Anniversaries

An anniversary belongs to a link of the built-in `spouse` kind, so both partners share one date. It is `YYYY-MM-DD`, or `--MM-DD` when the year is unknown. No other kind takes one and the attribute is refused (exit code 2).

```sh crm-example
crm person link person 1 2 --kind spouse --anniversary 2014-06-20
crm person link person 1 2 --kind spouse --anniversary=--06-20
crm person link person 1 2 --kind spouse --anniversary ""
crm person relate 1 2 --type spouse --anniversary 2014-06-20
```

An empty value clears it. `crm person show` lists it on the link (`anniversary`). MCP: `anniversary` on `crm_person_link`. A person merge keeps the date, and an undo of the merge restores it. In the terminal UI, picking Spouse in the Link form asks one more line, "Anniversary (blank skips)", and the date is a cell of the link row on the person page (`right` to the cell, `enter` to edit, `ctrl+z` undoes it).

## In the terminal UI

The Dates view (key 7) groups by month (OCTOBER), then by day with a divider: the date at the left and the relative time at the right. Below it are the birthdays, then the anniversaries of that day. A birthday row is the person with the age they turn at the right edge. An anniversary row reads "Joey-Hannah Osgood Anniversary" (or "Joey Osgood-Hannah Smith Anniversary" when the last names differ) with the years at the right edge. Both are blank when the year is unknown. Enter on an anniversary row opens the first partner. People who are archived and deleted links are left out.

`ctrl+o` has one row, Show: birthdays and anniversaries (the default), birthdays only, or anniversaries only. The view opens at today with the last 90 days above and the next 365 below. Scroll up to see past dates. `ctrl+shift+up` or `ctrl+shift+down` jumps back to today (`home` and `end` are the first and last row, and the fallback where a terminal does not report the chord). `ctrl+up` and `ctrl+down` jump a day. "Go to today" is also a palette entry.

The module key was `birthdays` before. A stored arrangement or key binding under that name moves to `dates` the first time the program reads `config.toml`. The old name is not accepted any more.

See also: keep-in-touch, links, relations, tui (Dates view, key 7), modules.
