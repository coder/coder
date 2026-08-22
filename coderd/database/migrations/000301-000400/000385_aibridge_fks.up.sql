-- We didn't add an FK as a premature optimization when the aibridge tables were
-- added, but for the initiator_id it's pretty annoying not having a strong
-- reference.
--
-- Since the aibridge feature is still in early access, we're going to add the
-- FK and drop any rows that violate it (which should be none). This isn't a
-- very efficient migration, but since the feature is behind an experimental
-- flag, it shouldn't have any impact on deployments that aren't using the
-- feature.

-- Step 1: Add FK without validating it
ALTER TABLE aibridge_interceptions
    ADD CONSTRAINT aibridge_interceptions_initiator_id_fkey
    FOREIGN KEY (initiator_id)
    REFERENCES users(id)
    -- We can't:
    -- - Cascade delete because this is an auditing feature, and it also
    --   wouldn't delete related aibridge rows since we don't FK them.
    -- - Set null because you can't correlate to the original user ID if the
    --   user somehow gets deleted.
    --
    -- So we just use the default and don't do anything. This will result in a
    -- deferred constraint violation error when the user is deleted.
    --
    -- In Coder, we don't delete user rows ever, so this should never happen
    -- unless an admin manually deletes a user with SQL.
    ON DELETE NO ACTION
    -- Delay validation of existing data until after we've dropped rows that
    -- violate the FK.
    NOT VALID;

-- Step 2: Drop existing interceptions that violate the FK.
DELETE FROM aibridge_interceptions
WHERE initiator_id NOT IN (SELECT id FROM users);

-- Step 3: Drop existing rows from other tables that no longer have a valid
--         interception in the database.
DELETE FROM aibridge_token_usages
WHERE interception_id NOT IN (SELECT id FROM aibridge_interceptions);

DELETE FROM aibridge_user_prompts
WHERE interception_id NOT IN (SELECT id FROM aibridge_interceptions);

DELETE FROM aibridge_tool_usages
WHERE interception_id NOT IN (SELECT id FROM aibridge_interceptions);

-- Step 4: Validate the FK
ALTER TABLE aibridge_interceptions
    VALIDATE CONSTRAINT aibridge_interceptions_initiator_id_fkey;
