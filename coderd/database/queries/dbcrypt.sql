-- name: GetDBCryptSentinelValue :one
SELECT val FROM dbcrypt_sentinel LIMIT 1;

-- name: SetDBCryptSentinelValue :exec
INSERT INTO dbcrypt_sentinel (val) VALUES ($1) ON CONFLICT (only_one) DO UPDATE SET val = excluded.val;
