INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
VALUES (
  'workspace_apps_token',
  1,
  'abc',
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  '2100-01-01 00:00:00 UTC'::timestamptz
);

INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
VALUES (
  'workspace_apps_api_key',
  1,
  'def',
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  '2100-01-01 00:00:00 UTC'::timestamptz
);

INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
VALUES (
  'oidc_convert',
  2,
  'ghi',
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  '2100-01-01 00:00:00 UTC'::timestamptz
);

INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
VALUES (
  'tailnet_resume',
  2,
  'jkl',
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  '2100-01-01 00:00:00 UTC'::timestamptz
);

