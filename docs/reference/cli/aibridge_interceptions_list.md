<!-- DO NOT EDIT | GENERATED CONTENT -->
# aibridge interceptions list

List AI Bridge interceptions as JSON.

## Usage

```console
coder aibridge interceptions list [flags]
```

## Options

### --initiator

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Only return interceptions initiated by this user. Accepts a user ID, username, or "me".

### --started-before

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Only return interceptions started before this time. Must be after 'started-after' if set. Accepts a time in the RFC 3339 format, e.g. "2006-01-02T15:04:05Z07:00".

### --started-after

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Only return interceptions started after this time. Must be before 'started-before' if set. Accepts a time in the RFC 3339 format, e.g. "2006-01-02T15:04:05Z07:00".

### --provider

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Only return interceptions from this provider.

### --model

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Only return interceptions from this model.

### --after-id

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

The ID of the last result on the previous page to use as a pagination cursor.

### --limit

|         |                  |
|---------|------------------|
| Type    | <code>int</code> |
| Default | <code>100</code> |

The limit of results to return. Must be between 1 and 1000.
