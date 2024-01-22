ALTER TABLE ONLY workspaces
ADD COLUMN favorite_of uuid DEFAULT NULL;
COMMENT ON COLUMN workspaces.favorite_of IS 'FavoriteOf contains the UUID of the workspace owner if the workspace has been favorited.';
