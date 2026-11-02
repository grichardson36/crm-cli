-- up

-- Order for the two lists that had none (owner request 2026-10-30, patterns/list-editor-standard).
-- Note types were listed by name and relation kinds by built-in first then id. The list editor
-- reorders every user-managed list with shift+up and shift+down, so both get a position. The
-- backfill keeps today's order. Deal stages and deal roles already have one (vocabulary.position).
-- The version a database gets is the position of the file, so never renumber.
ALTER TABLE note_types ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
UPDATE note_types SET position = (SELECT COUNT(*) FROM note_types n2 WHERE n2.name < note_types.name COLLATE NOCASE) + 1;

ALTER TABLE relation_kind ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
UPDATE relation_kind SET position = (
    SELECT COUNT(*) FROM relation_kind k2
    WHERE k2.object_type = relation_kind.object_type
      AND (k2.builtin > relation_kind.builtin OR (k2.builtin = relation_kind.builtin AND k2.id < relation_kind.id))
) + 1;

-- down

ALTER TABLE relation_kind DROP COLUMN position;
ALTER TABLE note_types DROP COLUMN position;
