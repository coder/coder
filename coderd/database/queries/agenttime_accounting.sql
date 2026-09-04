-- name: AccountAgentTimeMessages :one
SELECT account_agent_time_messages(@message_ids::bigint[])::bigint;

