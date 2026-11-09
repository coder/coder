-- Git refs belong to the root chat.
WITH subagent_statuses AS (
    DELETE FROM chat_diff_statuses cds
    USING chats c
    WHERE cds.chat_id = c.id
        -- Match Chat.IsSubChat when a deleted parent leaves
        -- only root_chat_id set.
        AND (c.root_chat_id IS NOT NULL OR c.parent_chat_id IS NOT NULL)
    RETURNING cds.*, COALESCE(c.root_chat_id, c.parent_chat_id) AS root_chat_id
)
INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    pull_request_state,
    changes_requested,
    additions,
    deletions,
    changed_files,
    refreshed_at,
    stale_at,
    created_at,
    updated_at,
    git_branch,
    git_remote_origin,
    pull_request_title,
    pull_request_draft,
    author_login,
    author_avatar_url,
    base_branch,
    pr_number,
    commits,
    approved,
    reviewer_count,
    head_branch
)
SELECT DISTINCT ON (root_chat_id, git_remote_origin, git_branch)
    root_chat_id,
    url,
    pull_request_state,
    changes_requested,
    additions,
    deletions,
    changed_files,
    refreshed_at,
    stale_at,
    created_at,
    updated_at,
    git_branch,
    git_remote_origin,
    pull_request_title,
    pull_request_draft,
    author_login,
    author_avatar_url,
    base_branch,
    pr_number,
    commits,
    approved,
    reviewer_count,
    head_branch
FROM subagent_statuses
ORDER BY
    root_chat_id,
    git_remote_origin,
    git_branch,
    -- Keep the most recently refreshed row from a subagent chat.
    refreshed_at DESC NULLS LAST,
    updated_at DESC,
    chat_id
-- Preserve the root chat's metadata and report times so moving duplicate
-- refs does not reorder refs the root chat already tracks.
ON CONFLICT (chat_id, git_remote_origin, git_branch) DO NOTHING;
