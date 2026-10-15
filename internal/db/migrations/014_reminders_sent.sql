-- up

-- Event reminders that were handed out (listed by `crm cal due-reminders`). The key is the event,
-- the start of the instance and the reminder, so a repeat listing stays empty and a moved
-- instance reminds again at its new time.
CREATE TABLE IF NOT EXISTS reminders_sent (
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    instance_start TEXT NOT NULL,
    minutes_before INTEGER NOT NULL,
    method TEXT NOT NULL,
    sent_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (event_id, instance_start, minutes_before, method)
) WITHOUT ROWID;

-- down

DROP TABLE IF EXISTS reminders_sent;
