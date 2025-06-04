-- name: InsertWormholeEvent :exec
INSERT INTO wormhole (id, created_at, event, event_type)
VALUES (gen_random_uuid(), now(), @event, @event_type);
