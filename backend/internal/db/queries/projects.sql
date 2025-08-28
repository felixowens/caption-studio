-- name: GetProject :one
SELECT (
    id,
    name,
    kind,
    parent_id,
    caption_provider,
    system_prompt,
    auto_caption_config
) FROM projects WHERE id = ?;