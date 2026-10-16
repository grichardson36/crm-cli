# Duplicates, merge and dismiss

Find people who are probably the same person, merge them, or say they are different.

## Find duplicates

Groups come highest confidence first. Levels: high (shared email, phone or social link), medium (same normalized name, the default minimum), low (nickname match, such as Bob and Robert), fuzzy (similar spelling).

```sh crm-example
crm dupes
crm dupes --min low
crm dupes --min high -f json
```

## Merge

`crm person merge KEEP DROP...` merges the drop people into the keep person. The survivor keeps its own values. An empty field takes the first non-empty value from the dropped people. Emails, phones, links, tags and custom fields are combined. Interactions, tasks, deals and relationships move to the survivor. Dropped people are archived with a pointer to the survivor, so you can restore them. Always preview first.

```sh crm-example
crm person merge 1 3 --dry-run
crm person merge 1 3 --dry-run -f json
crm person merge 1 3 --prefer title=3
```

`--prefer field=ID` takes one field from a specific person and can repeat. Fields: first_name, last_name, title, company, location, notes, summary, org_id, birthday, keep_in_touch, last_contacted_at, is_favorite, priority_tier, email, phone, or a link kind such as linkedin.

## Dismiss

If two people look alike but are different, dismiss the pair. It stops appearing in `crm dupes`.

```sh crm-example
crm dupes dismiss 1 2
crm dupes undismiss 1 2
```

See also: archive (restore a merged-away person), agents, tui-keys (Merge view).
