-- Superseded by one journal and one ledger per entity. These two were the
-- first thing written, before the patterns settled: one journal for every
-- entity rather than one for each, a single timestamp where two are required,
-- a row identifier where an entry identifier and a line number are required,
-- no lines at all, and a ledger with no state column.
DROP TABLE entity_journal;

DROP TABLE entity_ai_agents;
