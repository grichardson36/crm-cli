-- up

-- Attendees of an event who are not contacts: just the address and name the invitation carried.
-- Nobody is created from them. event_people stays the link to real contacts.
CREATE TABLE IF NOT EXISTS event_guests (
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'attendee' CHECK(role IN ('organizer', 'attendee')),
    rsvp TEXT NOT NULL DEFAULT 'needs-action' CHECK(rsvp IN ('needs-action', 'accepted', 'declined', 'tentative')),
    PRIMARY KEY (event_id, email)
) WITHOUT ROWID;

-- down

DROP TABLE IF EXISTS event_guests;
