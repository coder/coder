ALTER TABLE template_versions RENAME COLUMN git_auth_providers TO external_auth_providers;

ALTER TABLE git_auth_links RENAME TO external_auth_links;
