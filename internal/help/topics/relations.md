# Memberships, kinds and hierarchy

How people and organizations relate. All of it is a link (see links), so it works from either end.

## A person in many organizations

A membership has a free-text role, a start and end date (`YYYY`, `YYYY-MM` or `YYYY-MM-DD`, no end means current) and a primary flag. One live primary per person. The first current membership becomes primary. A past stay is dimmed in the terminal UI and a repeated stay at one organization is a second link (`--new`). Roles are free text, `crm roles` lists the ones in use.

```sh crm-example
crm person link org 2 1 --role Founder --from 2018 --to 2021
crm person link org 2 1 --role Advisor --from 2023 --new
crm roles
```

The vCard ORG on a phone is the primary organization, TITLE stays the person's title. An ORG you edit on the phone moves the primary membership to the organization of that name (case, punctuation and Inc/LLC endings ignored, merge aliases followed). A name that matches nothing creates the organization and then does the same, a person without a primary membership gets one, and a junk text such as `N/A` creates nothing. `people.company` takes the phone's text. Emptying ORG on the phone never removes a membership. See convert.

## Kinds between people

A kind reads "a is parent b", and seen from b the other label applies (child). Built in: related (the default), spouse, sibling, friend, colleague are mutual. parent/child, manager/report, mentor/mentee, referred-by/referred are directional. Noun kinds show the other person's role (a parent link shows child on the parent's page). Verb kinds (`--phrase`) read from the page you are on: A referred-by B shows "referred-by B" on A's page and "referred A" on B's. Naming the inverse (`--kind child`) reverses the ends.

```sh crm-example
crm relation-kind list
crm relation-kind add godparent --inverse godchild
crm relation-kind add neighbor
crm person link person 1 2 --kind friend
```

## Organization hierarchy

`crm org link org 1 2` makes 1 the parent of 2. An organization may have several parents and children. A link that would make an organization its own ancestor is refused. Add org kinds with `crm relation-kind add partner --type org`.

## Merge and aliases

`crm person merge` moves every link to the survivor. Only exact duplicate links are dropped (a membership is a duplicate when organization, role, start and end all match, so CTO and Advisor at one organization both stay), and the old id and uuid are kept as an alias so old references resolve.

See also: links, orgs, convert, duplicates, dav, people.
