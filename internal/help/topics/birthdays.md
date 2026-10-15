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

Fields: `person_id`, `name`, `date`, `days_until`, `turning`. `turning` is absent when the year is unknown. The default window is 30 days.

See also: keep-in-touch, tui (Birthdays view, key 5).
