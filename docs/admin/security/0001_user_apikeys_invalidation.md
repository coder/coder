# API Tokens of deleted users not invalidated

---

## Summary

Coder identified an issue in
[https://github.com/coder/coder](https://github.com/coder/coder) where API
tokens belonging to a deleted user were not invalidated. A deleted user in
possession of a valid and non-expired API token is still able to use the above
token with their full suite of capabilities.

## Impact: HIGH

If exploited, an attacker could perform any action that the deleted user was
authorized to perform.

## Exploitability: HIGH

The CLI writes the API key to `~/.coderv2/session` by default, so any deleted
user who previously logged in via the Coder CLI has the potential to exploit
this. Note that there is a time window for exploitation; API tokens have a
maximum lifetime after which they are no longer valid.

The issue only affects users who were active (not suspended) at the time they
were deleted. Users who were first suspended and later deleted cannot exploit
this issue.

## Affected Versions

All versions of Coder between v0.8.15 and v0.22.2 (inclusive) are affected.

All customers are advised to upgrade to
[v0.23.0](https://github.com/coder/coder/releases/tag/v0.23.0) as soon as
possible.

## Details

Coder incorrectly failed to invalidate API keys belonging to a user when they
were deleted. When authenticating a user via their API key, Coder incorrectly
failed to check whether the API key corresponds to a deleted user.

## Indications of Compromise

> [!TIP]
> Automated remediation steps in the upgrade purge all affected API keys.
> Either perform the following query before upgrade or run it on a backup of
> your database from before the upgrade.

Execute the following SQL query:

```sql
SELECT
  users.email,
  users.updated_at,
  api_keys.id,
  api_keys.last_used
FROM
  users
LEFT JOIN
  api_keys
ON
  api_keys.user_id = users.id
WHERE
  users.deleted
AND
  api_keys.last_used > users.updated_at
;
```

If the output is similar to the below, then you are not affected:

```sql
-----
(0 rows)
```

Otherwise, the following information will be reported:

- User email
- Time the user was last modified (i.e. deleted)
- User API key ID
- Time the affected API key was last used

> [!TIP]
> If your license includes the
> [Audit Logs](https://coder.com/docs/admin/audit-logs#filtering-logs) feature,
> you can then query all actions performed by the above users by using the
> filter `email:$USER_EMAIL`.
