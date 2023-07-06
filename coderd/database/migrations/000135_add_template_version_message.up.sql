ALTER TABLE template_versions ADD COLUMN message text NOT NULL DEFAULT '';

COMMENT ON COLUMN template_versions.message IS 'Message describing the changes in this version of the template, similar to a Git commit message. Like a commit message, this should be a short, high-level description of the changes in this version of the template. This message is immutable and should not be updated after the fact.';
