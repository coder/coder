# Sync settings

This document only discusses group sync settings at the current time.

## Group sync settings

The group sync settings configure how Coder uses IdP claims to populate its
group memberships. If so configured, Coder makes itself subordinate to the IdP.
The settings are parameters to a fixed algorithm which accomplishes this. Each
organization in a Coder deployment has its own, separate group sync settings.

### As a well-formed JSON text

- **W1. JSON text.** The text is a JSON text as defined by RFC 8259. Its value
  is an object.
- **W2. Unique member names.** No object in the text has two members with the
  same name.
- **W3. Possible members.** The top-level object may have only members with
  names drawn from the following list. Names are case-sensitive. Not every
  listed member need be present.
  - `field`
  - `mapping`
  - `regex_filter`
  - `auto_create_missing_groups`
  - `legacy_group_name_mapping`
- **W4. `field`.**
  - If present, its value is a string.
  - If empty (see below), then every other member must be empty.
- **W5. `mapping`.**
  - If present, its value is `null` or an object.
  - Every member name of `mapping` is a nonempty string.
  - The value of every member in `mapping` is an array of zero or more distinct
    strings, that is, with no string appearing twice.
  - Every string in every `mapping` array is a UUID in canonical text form: 36
    characters, lowercase hexadecimal digits in groups of 8-4-4-4-12 separated
    by hyphens. The nil UUID, `00000000-0000-0000-0000-000000000000`, must not
    be present.
- **W6. `regex_filter`.**
  - If present, its value is `null` or a string.
  - If a nonempty string, that string is a valid regular expression in RE2
    syntax.
  - When `regex_filter` is a nonempty string, it matches every member name of
    `mapping`. This match is for the regex as written; it may match on a
    substring.
- **W7. `auto_create_missing_groups`.**
  - If present, its value is `true` or `false`.
- **W8. `legacy_group_name_mapping`.** If present, its value is `null` or an
  empty object.

#### Equivalence and canonical forms

Two group sync settings texts are textually equal when their sequences of JSON
tokens are equal. Insignificant whitespace and the way strings are escaped are
ignored, so `"&"` and `"&"` are the same string; the order of members is
significant. Canonical form, below, is a property of the tokens, not of the
bytes.

All of the following are considered to be empty forms, depending on context. Not
all these forms are permissible for every member; only the ones that match the
type should be considered.
- An absent member.
- A member with value `null`.
- A member whose value is the empty string.
- A member whose value is the empty object.
- A member whose value is `false`.

The following are the canonical empty forms for each member:
- `"field": ""`
- `"mapping": {}`
- `"regex_filter": null`
- `"auto_create_missing_groups": false`
- `legacy_group_name_mapping` is absent

For member `legacy_group_name_mapping`, all its permissible values are
equivalent, which means this member has no nonempty values.

The canonical form of a nonempty member is the member itself. For `mapping`
specifically, its members are ordered lexicographically (by Unicode code point)
and their values are in their original order. (See the note on equivalence
below.)

The canonical form of a settings object is its members in canonical form in the
following order:
- `field`
- `mapping`
- `regex_filter`
- `auto_create_missing_groups`
- `legacy_group_name_mapping`

**Note:** Equivalence of canonical forms does not represent semantic
equivalence. That is, different canonical forms may configure the same behavior
of the settings algorithm. For settings texts that generate the same behavior to
have the same canonical form, the arrays within `mapping` would have to be put
in a set order (such as lexicographic). In addition, regular expressions that
match the same set of strings would have to be rewritten to a canonical form.
Even the algorithm to determine equivalence of regular expressions can have
exponential blowup, much less canonicalization. Lastly, all settings texts with
an empty `field` are semantically equivalent, whatever their other members hold;
W4 makes such a text ill-formed when any other member is nonempty. For all these
reasons, we do not implement a predicate function for semantic equivalence.

### As a valid JSON text for an organization

A group sync settings text is valid for an organization at a given moment when
every condition below holds. Validity depends on the state of the deployment
at that moment, not on the text alone.

- **V1. Well-formed as a JSON text.** The text satisfies W1 through W8. The
  conditions below apply only to a well-formed text.
- **V2. The organization exists.** The target organization exists and is not
  deleted.
- **V3. Every group exists.** Every UUID in every `mapping` array is the ID of
  an existing group.
- **V4. Every group is owned by the organization.** Every group named in
  `mapping` belongs to the target organization.
- **V5. No group is the Everyone group.** No UUID in `mapping` is the ID of the
  target organization's Everyone group, whose ID is the organization's ID. Every
  member of an organization is implicitly a member of its Everyone group.

### As the Go value used by sync

Group sync takes its settings as a Go value of type
`idpsync.GroupSyncSettings`, one per organization. The interpretation below
treats this value as its parameters. It has the same fields as
`codersdk.GroupSyncSettings`, the type used on the wire, and adds the methods
for storage and sync. There are three sources of this value, and they do not
carry the same guarantees.

- **A request.** The text is decoded into `codersdk.GroupSyncSettings` and
  passed through the rewriter; see [Request routes](#request-routes). The result
  is converted to the Go value and stored: in canonical form when it is valid
  or rewritten, and as received when it is not rewritten.
- **A stored row.** The Go value is decoded from the row keyed
  `<org-uuid>:group-sync-settings`, with no checks beyond JSON decoding. A row
  may fail W1 through W8, either because it was written before the current
  checks existed or because it was saved as received without a rewrite. No row
  keeps V1 through V5, because validity depends on the deployment's state at
  the moment of the check; see
  [Sources of misbehavior with group sync](#sources-of-misbehavior-with-group-sync).
  A row is checked only when it is fetched, or read by a PATCH endpoint before
  merging; see [Fetch route](#fetch-route). When a row fails to decode, sync
  uses the empty value, so group sync is skipped for that organization.
- **Deployment flags.** When no row exists for the default organization, the
  Go value is built from the deployment flags `CODER_OIDC_GROUP_*`. This value
  can include a nonempty `legacy_group_name_mapping`, and it passes through
  none of the checks. Every other organization without a row gets the empty
  value.

### The IdP input

Group sync takes one input from the IdP: the value of the claim named by
`field`, taken from the claims the IdP made about the user at their most
recent OIDC login.

Before group sync runs, the claim's value is resolved at login from more than
one source: currently the ID token and, depending on deployment configuration,
either the IdP's userinfo endpoint or the access token. Where the sources
disagree, one value takes precedence and the other is discarded. The details
are outside the scope of this document.

The claim value is converted to one of two results:

- **A list of IdP group names.** Group sync proceeds with the list.
- **Skip.** Group sync makes no change to the user's groups in this
  organization at this login, the same as when `field` is empty.

| Claim value                     | Result           | Effect on the org's groups    |
|---------------------------------|------------------|-------------------------------|
| Absent, `null`, or `""`         | Empty list       | Removed from all but Everyone |
| String without a comma          | One-element list | Synced to the list            |
| Array of strings                | Its elements     | Synced to the list            |
| String containing a comma       | Skip             | Unchanged                     |
| Array with a non-string element | Skip             | Unchanged                     |
| Any other type                  | Skip             | Unchanged                     |

"Removed from all but Everyone" is the special case of "Synced to the list"
when the list is empty. See
[Interpretation of the group sync settings](#interpretation-of-the-group-sync-settings)
for a full explanation.

### Interpretation of the group sync settings

This is a summary of the algorithm to which the settings act as parameters. The
algorithm runs once per organization at each login. It carries the list of IdP
group names from the IdP input through the stages below.

- **L0. Claim list**, a list of IdP group names. The list produced by the IdP
  input (assuming it doesn't resolve to a skip). The algorithm has the same
  result irrespective of order and the presence of any duplicates from here on.
- **L1. Renamed list**, a list of IdP group names. This step only occurs if
  `legacy_group_name_mapping` is nonempty, which per above is no longer
  well-formed. When the legacy mapping is empty, as W8 requires of a client's
  text, L1 equals L0. When it's not, L1 is L0 with each name that is a key of
  `legacy_group_name_mapping` replaced by that key's value.
- **L2. Filtered list**, a list of IdP group names. The names in L1 that
  `regex_filter` matches, by unanchored search. When `regex_filter` is empty,
  L2 equals L1.
- **M. Mapped contribution**, a set of UUIDs, possibly naming no group or a
  group in another organization. For each name in L2 that is a `mapping` key,
  every UUID in that key's array. An empty array contributes nothing, and the
  name goes no further.
- **N. Unmapped names**, a list of IdP group names. The names in L2 that are
  not `mapping` keys.
- **U. Unmapped contribution**, a set of groups in the organization. For each
  name in N, the organization's group whose name equals it exactly
  (case-sensitive). When `auto_create_missing_groups` is true, missing groups
  are created first; names with no such group contribute nothing.
- **T. Target set**, a set of groups in the organization. It consists of the
  Everyone group, every group in U, and every group in the organization whose
  UUID is in M. A UUID in M that names no group, or a group in another
  organization, is dropped and logged.

**Note:** These stages define the results of the algorithm, not the steps of the
code. The code may reach the same sets U and T by a different route.

The following postconditions hold at the commit of the login transaction.

- **P1. Skip makes no changes.** When the user is not a member of the
  organization, `field` is empty, or the claim converts to Skip, the user's
  group memberships in the organization are unchanged. Org sync runs first in
  the same transaction, so membership here means membership after org sync.
- **P2. Exact membership.** Otherwise, the set of the organization's groups the
  user belongs to equals T. Every membership outside T is removed, including
  ones an admin granted by hand.
- **P3. Auto-creation.** When `auto_create_missing_groups` is true, a group
  exists in the organization for every name contributing to U, created with
  source `oidc` if it did not exist. These groups persist even after the names
  stop arriving.
- **P4. Settings untouched.** Sync reads the settings and never writes them.
  Dormant members stay as stored.

These are postconditions, not invariants. After commit they can be falsified
by changes to membership, groups, settings, or the IdP, and the next login
re-establishes them.

### Sources of misbehavior with group sync

The goal of defining validity for sync settings is to ensure comprehensible
configuration and reliable operation. Group sync can misbehave in many ways,
some within the control of Coder itself, some outside of that control.

Misbehaviors within the control of Coder should be considered as future defects
in need of correction. Some of them might be considered present defects because
they violate security expectations. Others are not properly present defects
because they reflect inadequate product specification in the past. As product
specification places more demands for strictness on the behavior of group sync,
these defects will become ripe for correction.

The IdP is perforce outside the administrative control of Coder. Functioning
integration with an IdP entails that Coder make certain assumptions about the
behavior of that IdP. Unfortunately, we must operate in an environment with
incomplete information. The metadata discovery protocols available from IdPs do
not mandate information sufficient for Coder to make an adequately complete
model of IdP behavior. Thus we have no way to reliably verify our assumptions
about the IdP. The upshot is that we have to deal with ignorance about the IdP,
both initially and over time.

- Group sync settings might not be valid. In this case the settings do not
  satisfy some precondition to the sync algorithm, which might do something
  wrong as a result.
  - The settings might have previously been valid, but the criteria for validity
    were later strengthened. In this case the code was moving and an update did
    not mandate an upgrade of the settings.
    - **Cross-organization UUID saved.** It was previously possible to save
      settings where `mapping` contained a UUID for a group in a different
      organization, a violation of V4. Sync filters these out now, and a fetch
      removes them (R7).
    - **Everyone group saved.** It was previously possible to save settings
      where `mapping` contained the organization's Everyone group, a violation
      of V5. It has no effect, since T always contains the Everyone group, and
      relaxed mode removes it on save and on fetch (R5).
    - **Nonexistent UUID saved.** It was also previously possible that a
      `mapping` contained a UUID for a nonexistent or deleted group, a violation
      of V3. Sync drops it silently, and a fetch removes it (R6).
      - As a special case, the mapping-diff endpoint accepted a pair with no
        `Gets` and stored it as the nil UUID, a violation of W5. Sync drops it
        silently, just like any other UUID that names no group, and relaxed
        mode removes it on save and on fetch (R1).
    - **Regex filter versus mapping.** A filter can exclude IdP groups that the
      mapping still names. Such a name never survives into L2, so its `mapping`
      entry never contributes to M, and nothing reports it. This is a violation
      of W6. It is not rejected, but relaxed mode removes such members on save
      and on fetch (R4).
    - **Legacy mapping persisted from deployment flags.** For the default
      organization with no stored sync settings, the config and mapping-diff
      endpoints copy the legacy mapping built from `CODER_OIDC_GROUP_MAPPING`
      into the row they store, a violation of W8. Sync then applies it as the L1
      rename on every login.
    - **Stored forms outside W5.** The full-replace endpoint stored empty
      `mapping` keys, `null` arrays, and repeated UUIDs as it received them.
      Sync treats them harmlessly: repeated UUIDs collapse because T is a set,
      and a `null` array contributes nothing to M, the same as an empty one.
      Relaxed mode rewrites repeated UUIDs and `null` arrays on save and on
      fetch (R2, R3). An empty key is not rewritten, and is still saved as
      received.
  - The settings were valid previously but then the database changed. In this
    case the data was moving and the code did not keep the settings valid, by
    updating them automatically or by requesting user input if required.
    - **Group deleted after mapping.** When a group named within `mapping`
      existed at the time of saving those settings, deleting that group leaves
      its UUID in every mapping that names it, another violation of V3. The
      UUID stays in the saved settings until they are next fetched, when R6
      removes it.
    - **Group moved from one organization to another.** It is not currently
      possible for a group to change organizations, but that should not be
      relied on as holding in perpetuity. A move would leave every `mapping`
      that names the group in violation of V4.

- Group sync settings might be valid but wrongly applied. In this case sync
  acts, but the memberships it grants or removes are not the ones the settings
  were meant to produce. Wrongful grants come first below, then wrongful
  removals.
  - After internal Coder changes.
    - **Organization deleted.** Organization deletion is a soft delete
      (`UpdateOrganizationDeletedByID`) and leaves the
      `<org-uuid>:group-sync-settings` row behind. This is a violation of V2, so
      sync should not run. Yet under a very narrow set of conditions, not
      specified here, sync can still run on a deleted organization's settings.
    - **Group renamed.** Renaming is allowed (`UpdateGroupByID`) and UUID
      mappings are not affected by it. Mappings that point at a name break
      silently: the lookup of an N name in U, and the legacy rename in L1. At
      the next login the renamed group is no longer in T, so P2 removes its
      members, and with auto-create on, a new group is created under the old
      name.
  - After changes at the IdP.
    - **Issuer or tenant changed.** A new issuer or tenant can emit the same
      group names with different meanings. Coder has no record of which IdP a
      mapping key came from: the same strings in L0 select the same entries of
      M and U whatever they now mean.
    - **Claim name changed or removed.** If `field` no longer appears in the
      token or userinfo, the claim converts to an empty L0, so T is only the
      Everyone group, and P2 removes every other membership in the
      organization.
    - **Same name reused for a different group.** Coder can't tell two IdP
      groups apart if they emit the same string, since both arrive as the same
      element of L0. Whoever the IdP puts in the new group gets the old group's
      Coder memberships.
    - **IdP group recreated.** A recreated group with the same name inherits the
      old mapping, whoever its members are.
    - **Group overage or claim truncation.** Some IdPs leave groups out of the
      token past a size limit, or emit a pointer to them instead. Coder sees a
      partial or empty L0, so T shrinks and P2 removes the memberships that
      fell out of it.
    - **Groups claim moved between the ID token and userinfo.** Coder merges the
      two sources, so moving the claim or changing its scope can change L0. Its
      absence converts to an empty L0, the same as the user having no groups.
    - **IdP group renamed.** IdP group names appear as elements of list L0. If
      renamed, one or more of the following might occur:
      - It may not be picked up by the legacy rename, appearing unaltered in L1.
      - If `regex_filter` no longer matches it, it won't appear in L2.
      - It may no longer be a `mapping` key, so it moves from M to N and its
        mapped groups leave T.
      - With auto-create on, the new name gets a new group in U, per P3.
    - **Group identifier format changed.** Some IdPs have a single setting that
      changes the form of every value in the groups claim, for example between
      group object IDs and names, or between full group paths and bare names. It
      is a separate administrative action, but its effect is the same as
      renaming every IdP group at once; see **IdP group renamed**.

- Group sync settings might be valid but applied late. In this case sync applies
  the settings correctly, but only at the user's next login.
  - **Membership changed between logins.** Sync runs only at OIDC login, so P2
    is established only at the commit of a login. IdP removals do not reach
    Coder until the user's next login. Until that login, a user who keeps
    acting through an active session or an API token keeps their memberships
    and the access they grant. Refreshing the IdP token does not re-run group
    sync.

- Group sync settings might be valid but have no effect. In this case the
  settings look like sync should do something, but nothing happens.
  - **Deployment flags changed.** `CODER_OIDC_GROUP_*` apply only to the default
    organization, and only while no setting is stored for it
    (`AGPLIDPSync.GroupSyncSettings`). After the first save through the API they
    are silently ignored, and a change takes effect only at restart.
  - **Claim type changed.** `ParseStringSliceClaim` accepts only a string or a
    list of strings. Any other type of data converts to Skip, so by P1 the
    user's memberships in the organization are unchanged.
  - **IdP group deleted.** A deleted group's name no longer appears in L0, so
    its `mapping` entry contributes nothing to M; the entry is present but
    dormant. As expected, members drop out of the Coder group at their next
    login (but not before).

- Known code defects. These are misbehaviors that are not covered by the
  categories above.
  - **Lost update.** The full-replace endpoint reads and writes outside a
    transaction, while the other two use `ReadModifyUpdate`. Two concurrent
    writes can drop one admin's change without any error.
  - **Auto-created groups bypass group-name rules.** With
    `auto_create_missing_groups` on, IdP group names are inserted as Coder
    groups without the checks the API applies to a group created by hand. A name
    the API would reject, such as one containing a space or the reserved name
    `new`, becomes a group anyway.

- Known product specification defects. These are misbehaviors where the code
  does what it was designed to do, but the design itself produces a surprising
  or unsafe result. Fixing them means changing the specified behavior, not
  correcting the code.
  - **A missing claim strips memberships; a malformed claim preserves them.**
    When the claim named by `field` is absent, it converts to an empty L0 and P2
    removes the user from every group in the organization but Everyone. When
    the claim is present but malformed, it converts to Skip and P1 leaves every
    membership in place, so the less trustworthy input has the milder effect.

### Mitigation for legacy settings

The definitions of well-formed and valid sync settings are not in accord with
the current code. So that upgrading Coder does not suddenly break an existing
configuration, we introduce a relaxed mode of behavior. Relaxed mode keeps the
current behavior of the group sync endpoints, except where security requires a
new rejection, and except that a fetch may rewrite the stored row; see
[Request routes](#request-routes) and [Fetch route](#fetch-route). It relaxes
only where the current code is lax, and it is never less strict than the
current code. Relaxed mode is the only mode at present.

Strict mode is the direction of evolution. Relaxations will most likely be
removed one at a time rather than all at once. Each rewrite rule below is
therefore a separate relaxation with its own warning code, and any one of them
can become a rejection without changing the others.

The end goal of strict mode is that Coder never be a source of misbehavior. It
cannot eliminate misbehavior, both because the IdP is outside administrative
control and because we have some fundamental ignorance about it. What we can do,
however, is to ensure that no behavior inside Coder ever makes the sync settings
invalid.

#### Valid and legacy settings

Validity, as defined above, includes well-formedness (V1) and holds for an
organization at the moment of the check. It does not require canonical form.
In particular, valid settings have no legacy rename, since W8 allows
`legacy_group_name_mapping` only to be absent, `null`, or an empty object. The
legacy mapping is partway through being removed entirely.

Legacy settings are any that are not valid. This is not only a syntactic
requirement but also a semantic one, since validity demands coherence with the
database.

Strict mode will accept only valid settings, both in protocol and in the
database.

#### The rewriter

The rewriter is a function of a settings value and the state of the deployment
at the moment it runs. It does not depend on sync, so it can run wherever
settings are read or written. It returns one of three outcomes, together with
settings and a list of warnings.

- **Valid.** The input is valid. The settings returned are its canonical form,
  and there are no warnings.
- **Rewritten.** The input is not valid, and the rewrite rules make it valid.
  The settings returned are in canonical form, with one warning for each rule
  applied.
- **Not rewritten.** The input is not valid and is not rewritten; see
  [Settings that are not rewritten](#settings-that-are-not-rewritten). The
  settings returned are the input unchanged, with a warning saying why.

A rewrite must not change behavior. For every IdP input, sync on the rewritten
settings produces the same T as sync on the input, at the moment of the
rewrite.

#### Rewrite rules

Each rule is one relaxation. A rule either omits an element or modifies one.
An omission that empties a `mapping` array keeps the member with `[]`, because
a member with an empty array still keeps its name out of N.

- **R1. Nil UUID** (`nil_uuid`, omit). A `mapping` array contains the nil
  UUID, which W5 excludes. The rewrite removes it. No group has the nil UUID,
  so it contributes nothing to T. This covers a mapping-diff pair with no
  `Gets`.
- **R2. Repeated UUID** (`repeated_uuid`, modify). A `mapping` array contains
  the same UUID more than once, which W5 excludes. The rewrite keeps one copy.
  T is a set, so repeats have no effect.
- **R3. Null array** (`null_array`, modify). A member of `mapping` has the
  value `null`, where W5 requires an array. The rewrite replaces it with `[]`.
  Either way the name is a `mapping` key that contributes nothing to M.
- **R4. Key excluded by the filter** (`filter_excluded_key`, omit). A member
  name of `mapping` is not matched by `regex_filter`, whereas W6 requires every
  member name to match. The rewrite removes the member. Such a name never
  survives into L2, so the member never contributes to M.
- **R5. Everyone group** (`everyone_group`, omit). A `mapping` array contains
  the organization's Everyone group, which V5 excludes. The rewrite removes it.
  T always contains the Everyone group, so the UUID adds nothing.
- **R6. Missing group** (`missing_group`, omit, stored rows only). A `mapping`
  array contains a UUID that names no group, which V3 excludes. The rewrite
  removes it. Sync drops it before forming T, so it contributes nothing.
- **R7. Group in another organization** (`outside_group`, omit, stored rows
  only). A `mapping` array contains a UUID that names a group in another
  organization, which V4 excludes. The rewrite removes it. Sync drops it
  before forming T, so it contributes nothing.

#### Reporting

Responses carry the settings as saved or returned, together with a list of
warnings. Each warning gives:

- a JSON Pointer (RFC 6901) to the element concerned, such as `/mapping/eng/0`;
- the code of the rule or condition;
- the value received and the value accepted, if any.

The existing SDK methods for the GET endpoint and the three PATCH endpoints
keep their signatures and ignore the warnings. New SDK methods return them.
Violations of W2 and W3 are resolved by JSON decoding and are not reported.

#### Settings that are not rewritten

The rewriter checks all of these. If any holds, it does not apply any rewrite
rule. There is no such thing as a partial rewrite; either the rewrite makes the
settings valid or there is no rewrite at all.

- **Empty `field`** (`field_empty`). When `field` is empty and another member is
  not, the settings violate W4. Sync ignores the other members (P1), but the
  current code keeps them, so by policy the settings are kept as received.
  Strict mode will reject settings with an empty `field` and other nonempty
  members.
- **Legacy mapping** (`legacy_mapping`). A nonempty
  `legacy_group_name_mapping` violates W8. A rewrite that makes the settings
  valid is not feasible in general, because the legacy rename runs before the
  filter and its target is a group name, not a UUID.
- **Empty mapping key** (`empty_mapping_key`). A `mapping` key that is the
  empty string violates W5. An IdP claim that is an array can contain the
  empty string, so the key may apply, and removing it could change behavior.
- **Deleted organization** (`organization_deleted`, stored rows only). Stored
  settings for a deleted organization violate V2, and no omission repairs
  that.

#### Request routes

On the three PATCH endpoints, the rewriter runs on the settings the endpoint
would store: the request body for the full-replace endpoint, and the merged
result for the config and mapping-diff endpoints.

- **Valid** settings are saved in canonical form.
- **Rewritten** settings are saved as rewritten.
- **Not rewritten** settings are saved as received, with their warning, where
  the current code accepts them, and rejected otherwise.

Everything the current code rejects is still rejected: text that is not JSON,
members of the wrong JSON type, UUIDs that do not parse, regular expressions
that do not compile, and a nonempty legacy mapping sent to the full-replace
endpoint. In addition, a violation of V2, V3, or V4 is rejected; R6 and R7 do
not apply to requests. A legacy mapping echoed back from a GET is treated the
same as one written anew.

Two behaviors of the current UI interact with these rules. The UI never sends
`legacy_group_name_mapping`, so a save from the UI through the full-replace
endpoint removes a stored legacy mapping, as it does today. The UI sends
`"regex_filter": ""` when no filter is set. That is well-formed, and saving
it in canonical form stores `null`.

#### Fetch route

On GET, the stored row is passed through the rewriter inside `ReadModifyUpdate`.
When the outcome is rewritten, the row is replaced with the rewrite, and the
write is attributed to a system actor, not to the user who made the request.
Otherwise the row is left unchanged. GET returns the canonical form of the row
as stored at the moment of return; a row that is only non-canonical is not
rewritten. The outcome never makes GET fail.

The PATCH endpoints read the existing row through the same path before merging,
so the config and mapping-diff endpoints merge into rewritten settings. A value
built from deployment flags, when no row exists, is not rewritten.

Sync reads rows as they are and does not rewrite them. Rewriting at sync would
be safe, since a rewrite does not change behavior, but it would defeat
reporting. Warnings arise only when a rewrite happens; once a row has been
rewritten, every later fetch finds it valid and reports nothing. Sync
runs at login, where there is no channel to an administrator, so its warnings
would reach only the server log. The administrator would next see corrected
settings with no account of what changed, beyond the audit record of a system
write. Rewriting on fetch delivers the warnings to the person looking at the
settings. Sync also runs inside the transaction of every login, and writing
settings rows there would add writes and contention to logins for no change in
behavior.
