-- Both reads filter on a (type, identifier) pair and order by id. Carrying id
-- in the index lets the ordering come from the index rather than a sort.
CREATE INDEX idx_entity_journal_subject ON entity_journal (subject_type, subject, id);

CREATE INDEX idx_entity_journal_actor ON entity_journal (actor_type, actor, id);
