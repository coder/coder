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

## What is not done

- **Chat tools are only partly held to the identity's scopes.** Creating,
  starting and stopping a workspace are. Listing and reading templates, and
  the chat's own bookkeeping, run with the root user's roles. Holding every
  tool to the scopes needs chats to tell a tool's action from their own
  bookkeeping.
- **Nothing creates an identity for a chat by itself.** The caller names the
  identity as the owner. A person's chat list does not show the chats of
  their identities.
- **A chat or workspace of an identity has none of the person's
  credentials.** No Git or other external sign-in, no provider keys of the
  person's, no tokens for MCP servers. A template that requires an external
  sign-in refuses an identity. This is the subject of the second document,
  on access to other systems.
- **Limits that no delegated identity can exceed** are not built.
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
