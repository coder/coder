ALTER TABLE ONLY workspaces
ADD COLUMN favorite boolean NOT NULL DEFAULT false;
COMMENT ON COLUMN workspaces.favorite IS 'Favorite is true if the workspace owner has favorited the workspace.';
