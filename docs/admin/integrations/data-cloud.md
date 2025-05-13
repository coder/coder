# DX Data Cloud

[DX](https://getdx.com) is a developer intelligence platform used by engineering
leaders and platform engineers.

You can tag your Coder users in DX Data Cloud to filter and analyze user activity data to:

- Measure adoption and impact
- Identify feature usage patterns
- Personalize user experiences
- Proactively address issues

## Requirements

- A DX Data Cloud subscription
- Access to Coder user data through the Coder CLI, Coder API, an IdP, or and existing Coder-DX integration
- Coordination with your Data Cloud Customer Success Manager

## Extract Your Coder User List

<div class="tabs">

You can use the Coder CLI, Coder API, or your Identity Provider (IdP) to extract your list of users.

If your organization already uses the Coder-DX integration, you can find a list of active Coder users directly within DX.

### CLI

Use `users list` to export the list of users to a CSV file:

```shell
coder users list > users.csv
```

Visit the [users list](../../reference/cli/users_list.md) documentation for more options.

### API

Use [get users](../../reference/api/users#get-users):

```bash
curl -X GET http://coder-server:8080/api/v2/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

To export the results to a CSV file, you can use the `jq` tool to process the JSON response:

```bash
curl -X GET http://coder-server:8080/api/v2/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY' | \
  jq -r '.users | (map(keys) | add | unique) as $cols | $cols, (.[] | [.[$cols[]]] | @csv)' > users.csv
```

Visit the [get users](../../reference/api/users.md#get-users) documentation for more options.

### IdP

If your organization uses a centralized IdP to manage user accounts, you can extract user data directly from your IdP.

This is particularly useful if you need additional user attributes managed within your IdP.

</div>

## Engage your DX Data Cloud Customer Success Manager

Provide the file to your dedicated DX Data Cloud Customer Success Manager (CSM).

Your CSM will:

1. Import the CSV file into the Data Cloud platform
1. Associate Coder user identifiers with corresponding records in your Data Cloud environment
1. Create the necessary links between your Coder users and their activity data

## Use Coder as a Data Cloud Filter

After the tagging process is complete, you'll have a **Coder** filter option within your Data Cloud dashboards,
reports, and analysis tools that you can use to:

- Segment your data based on Coder usage
- Filter by additional user attributes that are included in your CSV file
- Perform granular analysis on specific segments of your Coder user base
- Understand unique behaviors and patterns across your Coder users

## Related Resources

- [DX Data Cloud Documentation](https://help.getdx.com/en/)
- [Coder API Documentation](../../reference/api/users)
- [PlatformX Integration](./platformx.md)
