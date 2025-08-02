# DX

[DX](https://getdx.com) is a developer intelligence platform used by engineering
leaders and platform engineers.

DX uses metadata attributes to assign information to individual users.
While it's common to segment users by `role`, `level`, or `geo`, it’s become increasingly
common to use DX attributes to better understand usage and adoption of tools.

You can create a `Coder` attribute in DX to segment and analyze the impact of Coder usage on a developer’s work, including:

- Understanding the needs of power users or low Coder usage across the org
- Correlate Coder usage with qualitative and quantitative engineering metrics,
  such as PR throughput, deployment frequency, deep work, dev environment toil, and more.
- Personalize user experiences

## Requirements

- A DX subscription
- Access to Coder user data through the Coder CLI, Coder API, an IdP, or an existing Coder-DX integration
- Coordination with your DX Customer Success Manager

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

Use [get users](../../reference/api/users.md#get-users):

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

## Contact your DX Customer Success Manager

Provide the file to your dedicated DX Customer Success Manager (CSM).

Your CSM will import the CSV of individuals using Coder, as well as usage frequency (if applicable) into DX to create a `Coder` attribute.

After the attribute is uploaded, you'll have a Coder filter option within your DX reports allowing you to:

- Perform cohort analysis (Coder user vs non-user)
- Understand unique behaviors and patterns across your Coder users
- Run a [study](https://getdx.com/studies/) or setup a [PlatformX](https://getdx.com/platformx/) event for deeper analysis

## Related Resources

- [DX Data Cloud Documentation](https://help.getdx.com/en/)
- [Coder CLI](../../reference/cli/users.md)
- [Coder API](../../reference/api/users.md)
- [PlatformX Integration](./platformx.md)
