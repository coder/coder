# What the proof of concept showed

## Checks

[validate.py](validate.py) ran against a deployment with this branch, a
template that keeps three workspaces ready, and a k3s cluster. 21 of 21
checks passed.

| Check | What it shows                                                                                              |
|-------|------------------------------------------------------------------------------------------------------------|
| A1    | Creating an identity returns a record under the person, with no scopes and no credential                    |
| A2    | Somebody who is not above the identity cannot see it, change it, or add below it or its parent              |
| B1    | Somebody who is not above the identity cannot create a workspace on its behalf                              |
| B2    | The unchanged command line creates it: `coder create <identity>/<name>`                                     |
| B3    | The identity owns the workspace and the person is who asked                                                 |
| B4    | Its daemon connects although the identity may do nothing with the API (ready about 6 s after the request)   |
| B5    | The person can enter it, and what runs in it is the identity to Coder                                       |
| C1    | With no scopes, the credential in its workspace is worth nothing                                            |
| C2    | The identity cannot raise itself                                                                            |
| C3    | Raising it applies to the credential it already has, and what its parent owns stays out of reach            |
| C4    | What it was not given stays refused                                                                         |
| D1    | An identity creates an identity below itself, and can hand on no more than it holds                         |
| D2    | It creates a workspace on behalf of the identity below it, which the person can enter as well               |
| D3    | Lowering the parent lowers the identity below it at once                                                    |
| E1    | With permission across the organization, a service creates an identity under a member it is not above       |
| E2    | It builds the session's workspace on behalf of the identity, and the member can enter it                    |
| E3    | When the session ends it deletes the workspace and the identity                                             |
| F1    | An identity that owns a workspace or has an identity below it cannot be deleted                             |
| F2    | The person deletes the identities' workspaces on their behalf                                               |
| F3    | Deleting an identity ends its credentials                                                                   |
| F4    | The audit log names who created, changed and deleted the identities                                         |

The Go tests cover the same ground without a cluster, and three things the
script does not: a chat that runs under an identity, quota that counts
against the root user, and the cleanup of identities nothing depends on.

Two more things were checked by hand, on a development server.

With the provider branch, a template read `delegated = true`, the parent, the
root user and the labels for workspaces of a delegated identity and of a
delegated identity below it, and `delegated = false`, no parent, no labels and
the owner as its own root user for a person's workspace.

The token a template hands to a workspace is held to the scopes of the
delegated identity that owns it. A member created two delegated identities,
one with the `template:read` scope and one with no scopes, and a workspace on
behalf of each from a template that puts
`data.coder_workspace_owner.me.session_token` in the workspace. The member
then entered both over SSH. In the first, `coder templates list` showed the
templates the member sees. In the second it showed none, and fetching a
template by its ID answered 404. Listing is not refused without the scope:
Coder answers with an empty list. The token in each workspace belonged to the
delegated identity. Swapping the scopes of the two identities swapped the
results on the next request, with the same tokens and no rebuild.

## What had to change that was not expected

- **A chat is owned by the identity, not by the person.** Chats attribute
  everything to their owner, so making the identity the owner needed no
  change to how chats run. The person then works with the chat on behalf of
  the identity, which every chat route now allows for.
- **Deleting a user and suspending a user had to know about identities.** An
  identity acts with the roles of its root user. A suspended root user
  suspends its identities, and a user with identities cannot be deleted.
- **Quota had to follow the root user.** An identity is in no group, so it
  would have had no quota, or the organization's default a second time.
- **AI seats had to follow the root user as well.** A seat is taken by
  whoever makes a model call, so every delegated identity a person runs
  would have taken a seat of its own.

## What is not done

- **A chat's tools are not held to the identity's scopes.** A chat creates,
  starts and stops its workspace on behalf of the identity, for whoever sent
  its latest message, so the identity needs no scopes for it and the sender
  is recorded as who asked. Listing and reading templates, and the chat's own
  bookkeeping, run with the root user's roles. The scopes decide only when
  the identity sent the message itself, or the sender may not use it.
- **Nothing creates an identity for a chat by itself.** The caller names the
  identity as the owner. A person's chat list does not show the chats of
  their identities.
- **A chat or workspace of an identity has none of the person's
  credentials.** No Git or other external sign-in, no provider keys of the
  person's, no tokens for MCP servers. A template that requires an external
  sign-in refuses an identity. This is the subject of the second document,
  on access to other systems.
- **The groups and roles a template is given are the identity's own**, not
  its root user's. A template that decides by the owner's groups sees none
  of the root user's.
- **The form for a workspace's parameters does not know that the owner is a
  delegated identity.** The new owner attributes are set when the workspace
  is built. The form's preview comes from a separate module that would have
  to learn them.
- **Only an owner can create an API token for a delegated identity.** Such a
  token is held to the identity's scopes and its creation is audited, but
  the identity's parent cannot create one yet.
- **A second daemon under an identity inside a person's workspace** (level
  1 of the maturity model) is not built.
- **Identities show up wherever accounts do**, for example in the list of
  users. They are service accounts, which need a Premium license that is not
  checked here.
- **Entering an identity's workspace relies on workspace sharing.** Where a
  deployment turns sharing off, the accounts above an identity can still
  build and delete its workspaces, and cannot enter them.
- **Spend counting against the root user's budget is built and not
  checked**: the model the tests use reports no cost.
- **The audit log names the identity and not its root user.** The record
  that ties the two together is kept after the identity is deleted.
- **An identity is in one organization.**
- **Suspending an identity does not suspend the identities below it.**
- **Agent Relay itself is unchanged.**
