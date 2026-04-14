-- name: FavoriteTemplate :exec
INSERT INTO template_favorites (user_id, template_id, created_at)
VALUES (@user_id, @template_id, NOW())
ON CONFLICT DO NOTHING;

-- name: UnfavoriteTemplate :exec
DELETE FROM template_favorites
WHERE user_id = @user_id AND template_id = @template_id;

-- name: GetUserTemplateFavorites :many
SELECT template_id FROM template_favorites
WHERE user_id = @user_id;
