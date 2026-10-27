# Birthdays

Store a birthday on a person and list the upcoming ones.

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

## In the terminal UI

The Birthdays view groups by month (OCTOBER), then by day with a divider: the date at the left and the relative time at the right, the people below it with the age they turn at the right edge (blank when the year is unknown). It opens at today with the last 90 days above and the next 365 below. Scroll up to see past birthdays. `ctrl+shift+up` or `ctrl+shift+down` jumps back to today (`home` and `end` are the first and last row, and the fallback where a terminal does not report the chord). `ctrl+up` and `ctrl+down` jump a day. "Go to today" is also a palette entry.

See also: keep-in-touch, tui (Birthdays view, key 7), modules.
