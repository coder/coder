## Problem Statement

Coder needs to support Linear as a first-class External Auth Provider so an already authenticated Coder user can connect their Linear account. The immediate product need is Linear User Mapping: when future Linear integrations receive a Linear actor ID, Coder must be able to resolve that Linear user ID to the Coder user who connected Linear.

Today Coder can store OAuth tokens for generic external auth providers, but it does not have a generic, reliable way to store an External User Identity for non-GitHub providers. A generic Linear OAuth configuration can acquire a token, but it cannot guarantee that Coder has captured Linear `viewer.id`, cannot expose a string-based identity in the external auth APIs, and cannot provide a deterministic lookup from Linear actor ID to Coder user.

## Solution

Add Linear as a First-Class External Auth Provider. A Coder user will connect Linear through the existing external auth flow. During the OAuth callback, Coder will exchange the code for a token, call Linear GraphQL `viewer` with that token, and persist the Linear user ID plus minimal External User Display Metadata on the external auth link. Future Linear integrations can resolve Linear webhook actor IDs by looking up the external auth link by provider ID and Linear user ID.

Linear external auth is not a Primary Login Method and does not add Linear SSO for Coder. A user must already be authenticated to Coder before connecting Linear.

The feature will preserve existing external auth token behavior. Linear access tokens will be available through the same workspace token surfaces as other External Auth Providers, while Linear identity metadata will be exposed through control-plane external auth APIs only.

## User Stories

1. As a Coder administrator, I want to configure Linear as an External Auth Provider, so that users can connect their Linear accounts to Coder.
2. As a Coder administrator, I want Linear to have built-in OAuth defaults, so that I do not need to manually configure every Linear endpoint.
3. As a Coder administrator, I want Linear external auth to use the least privileged default scope, so that users are not asked to grant write permissions before an integration needs them.
4. As a Coder administrator, I want to override Linear scopes, so that future integrations can request additional Linear permissions when needed.
5. As a Coder administrator, I want Linear scopes to be encoded in the format Linear expects, so that multi-scope configurations work correctly.
6. As a Coder administrator, I want the Linear OAuth callback URL to follow the existing external auth provider ID pattern, so that Linear setup is predictable.
7. As a Coder administrator, I want Linear external auth documentation to state that this is not Coder SSO, so that I do not confuse it with a Primary Login Method.
8. As a Coder administrator, I want Linear setup documentation with a minimal environment example, so that I can configure the provider without reverse engineering Coder settings.
9. As a Coder administrator, I want Linear token revocation on unlink, so that disconnecting Linear from Coder also attempts to revoke the Linear OAuth grant.
10. As a Coder administrator, I want revocation errors to be surfaced without preserving the Coder link, so that users can disconnect locally even if Linear revocation fails.
11. As a Coder administrator, I want deleting a Coder user to remove their External Auth Provider connections, so that deleted users do not retain OAuth credential records.
12. As a Coder administrator, I want a Linear user ID to map to at most one Coder user per provider ID, so that future webhook actor resolution is deterministic.
13. As a Coder administrator, I want deleted users not to block future Linear User Mappings, so that a Linear account can be reconnected after a Coder account is deleted.
14. As a Coder administrator, I want multiple Linear providers to be supported by unique provider IDs, so that different Linear apps or environments can be configured independently.
15. As a Coder administrator, I want Linear identity mapping to be keyed by provider ID and Linear user ID, so that mappings do not collide across multiple Linear providers.
16. As a Coder user, I want to connect Linear from Coder's existing external auth UI, so that I can authorize Coder to use my Linear identity and token.
17. As a Coder user, I want Coder to show which Linear account is connected, so that I can confirm I connected the correct Linear account.
18. As a Coder user, I want Coder to fail the connection if it cannot fetch my Linear user ID, so that I am not left in a connected but unmapped state.
19. As a Coder user, I want a clear error if Linear identity lookup fails, so that I know to retry connecting Linear.
20. As a Coder user, I want Coder to prevent silent transfer of a Linear User Mapping, so that my Linear account is not accidentally associated with a different Coder user.
21. As a Coder user, I want a clear reconnect requirement if Coder detects a Linear account mismatch, so that I can repair the connection intentionally.
22. As a Coder user, I want Coder to keep my Linear display metadata reasonably fresh, so that connected account UI remains useful after I change my name or avatar.
23. As a Coder user, I want my Linear email to be used only as display metadata, so that identity mapping is based on the stable Linear user ID.
24. As a Coder user, I want my Linear token to be available through existing external auth token mechanisms, so that workspace templates and tools can use Linear integrations as me.
25. As a Coder user, I do not want my Linear identity metadata unnecessarily exposed to workspace token responses, so that PII exposure does not expand beyond the control-plane external auth APIs.
26. As a template author, I want to use the existing external auth token data source for Linear, so that templates can call Linear APIs with the connected user's token.
27. As a template author, I want Linear to behave like other external auth providers for token access, so that I do not need provider-specific Coder template logic.
28. As a future Linear integration developer, I want to resolve a Linear webhook actor ID to a Coder user ID, so that integration actions can be associated with the correct Coder user.
29. As a future Linear integration developer, I want a lookup by provider ID and external user ID, so that webhook handling can be deterministic.
30. As a future Linear integration developer, I want Linear external auth to use user actor tokens, so that the connected Linear user remains the identity being mapped.
31. As a future Linear integration developer, I want app actor credentials to remain separate from user external auth, so that service automation does not confuse user mapping.
32. As an API client, I want external auth responses to include a string-based identity object, so that providers with non-numeric IDs are represented correctly.
33. As an API client, I want the existing legacy external auth user field to remain compatible, so that GitHub-related clients are not broken by Linear support.
34. As an API client, I want list external auth responses to include stored identity metadata, so that settings UIs can show connected accounts without provider calls.
35. As an API client, I want single-provider external auth detail to perform explicit Linear validation, so that I can get fresh identity state when needed.
36. As a reviewer, I want a documented schema decision, so that I can verify why identity fields live on external auth links instead of another table.
37. As a reviewer, I want automated tests around Linear callback, validation, uniqueness, and cleanup behavior, so that the implementation can be verified without live Linear credentials.
38. As a reviewer, I want a dogfooding plan using real Linear credentials when available, so that the OAuth setup is proven end to end.
39. As a reviewer, I want a mocked Linear fallback dogfood path, so that validation can still proceed when live Linear credentials are unavailable.
40. As a maintainer, I want Linear identity fetching to be isolated behind a small module, so that GraphQL response parsing and error handling can be tested without running the full server.
41. As a maintainer, I want Linear-specific behavior to be explicit rather than auto-detected from token extras, so that External User Identity remains trustworthy.
42. As a maintainer, I want generic OAuth providers not to populate identity fields implicitly, so that accidental or unstable provider values are not treated as identity.
43. As a maintainer, I want GraphQL errors from Linear to fail identity validation, so that Coder does not accept ambiguous partial identity responses.
44. As a maintainer, I want missing optional Linear display fields to be tolerated, so that the mapping still succeeds when only identity is available.
45. As a maintainer, I want Coder user deletion to clean up external auth links, so that credential cleanup is consistent with related user-linked auth data.

## Implementation Decisions

- Linear will be added as a First-Class External Auth Provider, not as a Primary Login Method.
- Linear will not be added to any Git-provider matching behavior.
- Linear will use built-in defaults for authorization URL, token URL, revocation URL, display name, icon, API base URL, and default scopes.
- Linear default scope will be `read` only. Additional scopes must be configured explicitly by an administrator.
- Linear OAuth scope encoding will be provider-specific and comma-separated to match Linear's OAuth contract.
- Linear will use the default user actor. App actor behavior and client credentials are out of scope for user external auth mapping.
- Linear will require a client secret for Coder's server-side OAuth flow unless Linear's official confidential-client behavior proves otherwise during implementation.
- Linear URL overrides will remain possible through existing external auth configuration. Linear identity fetch will use the configured API base URL plus the GraphQL endpoint path.
- Linear identity validation will be implemented by calling Linear GraphQL `viewer`, not by using a generic GET validation URL.
- A provider-aware validation capability should report Linear as supporting validation even without a generic validation URL.
- Callback handling for Linear must fetch Linear `viewer.id` before storing the connection. If identity cannot be fetched, the connection fails and no connected Linear link is stored.
- Explicit single-provider validation will fetch Linear `viewer`, verify the stored mapping, and refresh display metadata if the ID matches.
- Token refresh will remain token-only and will not call Linear GraphQL.
- List external auth responses will not call Linear. They will use stored identity and lightweight token status behavior.
- The external auth link will be the source of truth for Linear User Mapping.
- Generic identity and display metadata fields will be added to external auth links. These fields will include external user ID, login, name, email, and avatar URL.
- The external user ID field will be non-null text with an empty string default, following existing Coder patterns for linked identities.
- A partial uniqueness constraint will ensure that a provider ID and non-empty external user ID can appear at most once.
- The mapping lookup will use provider ID and external user ID, not provider type alone.
- A database lookup by provider ID and external user ID will be added for future Linear webhook actor resolution.
- The lookup will return the external auth link. Callers are responsible for fetching and applying Coder user status policy.
- Deleting a Coder user will remove their external auth links through the existing soft-delete cleanup mechanism.
- A foreign key from external auth links to users will not be added as part of this feature, to keep migration risk bounded.
- GitHub external auth will not be updated to populate the new generic identity fields in this feature.
- Generic OAuth providers will not automatically populate identity fields from token extras or common key names.
- Only explicit first-class provider identity fetchers can populate External User Identity fields.
- External auth API responses will add a new optional `identity` object using string IDs.
- The legacy provider `user` response shape will remain for backward compatibility.
- List external auth responses will include stored `identity` metadata.
- Single-provider external auth responses will return fresh `identity` metadata after explicit validation.
- Linear identity metadata will be exposed through control-plane external auth APIs to callers authorized to read the external auth link.
- Linear identity metadata will not be added to workspace token responses in the initial implementation.
- Linear tokens will remain available through existing external auth token surfaces.
- Linear unlink will use the existing delete-then-revoke behavior. If revocation fails, the Coder link remains deleted and the API reports the revocation error.
- Linear GraphQL parsing will be strict for identity and tolerant for display metadata. HTTP failures, GraphQL errors, null viewer, and empty ID fail. Missing optional display fields become empty values.
- If explicit validation returns a different Linear user ID than the stored mapping, Coder will not mutate the mapping. The link is considered invalid and the user must reconnect.
- No active backfill job will be created. Existing Linear-like links can be healed by explicit validation or user reconnection.
- Linear device flow is out of scope unless Linear documents OAuth device authorization support.
- A local Linear icon should be bundled if brand and license usage is acceptable. If not, the provider should use a generic icon until a compliant asset is available.
- An ADR records the decision to store external auth user identity on external auth links.

## Testing Decisions

- Tests should validate external behavior and durable contracts rather than private implementation details.
- The Linear identity client should be tested as a deep module with mocked GraphQL responses. Good tests cover success, HTTP failure, GraphQL `errors`, null viewer, empty ID, and missing optional display fields.
- Provider default tests should cover Linear OAuth defaults, display metadata defaults, API base URL behavior, default scope, and provider-specific scope encoding.
- Callback tests should cover successful token exchange plus identity persistence, failed identity fetch, missing identity ID, and duplicate Linear user mapping rejection.
- Explicit validation tests should cover filling missing identity, detecting mismatched identity, updating display metadata when the ID matches, and returning the new API `identity` shape.
- List endpoint tests should cover returning stored identity without requiring a live provider validation call.
- Database tests should cover identity columns, partial uniqueness, lookup by provider ID and external user ID, and cleanup when a Coder user is soft-deleted.
- Revocation tests should cover Linear using provider revocation by default and preserving existing delete-then-revoke API behavior when revocation fails.
- Token response tests should verify that Linear access tokens remain available through existing workspace token surfaces and that identity metadata is not added to workspace token responses.
- API compatibility tests should verify that the legacy external auth `user` field remains compatible and that the new `identity` field supports string IDs.
- Documentation tests or checks should ensure Linear setup docs mention that Linear external auth is not Coder SSO.
- Dogfooding should use a real Linear OAuth app when credentials are available. The reviewer evidence should include screenshots and video of provider setup, consent, connection, identity display, API or database mapping, and unlink behavior.
- If live Linear credentials are unavailable, dogfooding should use a mocked Linear OAuth and GraphQL server and record the credential blocker explicitly.
- Prior art exists in existing external auth default, callback, validation, revocation, API route, database migration, and user deletion cleanup tests.

## Out of Scope

- Linear as a Primary Login Method for Coder.
- Linear SSO, OIDC, or SAML login to Coder.
- Linear device flow support.
- Linear app actor tokens or client credentials for service automation.
- Linear webhook ingestion or full Linear integration workflows.
- Generic admin-configured identity mapping for arbitrary OAuth providers.
- Automatically populating generic identity fields for GitHub external auth.
- Adding a foreign key from external auth links to users.
- Storing Linear organization or workspace identity on the external auth link.
- Exposing Linear identity metadata through workspace token responses.
- Active background backfill of existing external auth links.

## Further Notes

Acceptance criteria:

- A configured Linear External Auth Provider can be connected by an already authenticated Coder user.
- Coder persists the Linear user ID on the external auth link before considering the connection complete.
- The same Linear user ID cannot be connected to two Coder users for the same provider ID.
- Future code can look up an external auth link by provider ID and Linear user ID.
- The control-plane external auth APIs expose a string-based identity object.
- Workspace token surfaces continue to provide the Linear access token without adding identity metadata.
- Deleting a Coder user removes their external auth links.
- Documentation makes clear that Linear external auth is not Coder SSO.
- Automated tests and dogfooding evidence demonstrate the behavior above.

Dogfooding plan:

- Start a local Coder dev server with Linear external auth configured.
- Configure a Linear OAuth app with the callback URL derived from the provider ID.
- Connect Linear from the Coder UI.
- Capture screenshots and video for the Coder auth prompt, Linear consent, connected state, identity API response, stored mapping, and unlink behavior.
- Verify a mocked or real future-style lookup from Linear actor ID to Coder user ID through provider ID and external user ID.
- If live Linear credentials are unavailable, use a mocked OAuth and GraphQL server and document the limitation.

Related ADR:

- Store external auth user identity on external auth links.

---
_Generated with [`mux`](https://github.com/coder/mux) • Model: `openai:gpt-5.5` • Thinking: `high`_
