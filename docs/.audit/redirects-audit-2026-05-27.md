# Redirects audit: TS/TSX docs-URL references

Generated: 2026-05-27T15:43:00.186Z

Tracks: [DOCS-253](https://linear.app/codercom/issue/DOCS-253) (parent: [DOCS-209](https://linear.app/codercom/issue/DOCS-209)).

## Method

This audit cross-references every static `docs("...")` call, every `docs(\`...\`)` template literal with a literal prefix, and every hardcoded `coder.com/docs/...` URL in TS/TSX files against the source side of every `/docs/*` rule in `coder/coder.com/redirects.json`. Anything that matches a redirect source is stale and needs to be updated to the destination.

Source of truth for the redirect set: `/home/coder/coder.com/redirects.json` at audit time.

Scanned roots:

* `/home/coder/coder/site/src`
* `/home/coder/coder.com/src`

Pattern matchers:

* `docs("/...")` and `docs('/...')` and `docs(\`/...\`)` (no `${}`).
* `docs(\`/.../$\{expr\}/...\`)` (literal prefix only; flagged as dynamic for manual review).
* Any string literal containing `https://coder.com/docs/...` or `https://*.coder.com/docs/...`.

Hash fragments (`#anchor`) and query strings (`?foo`) are stripped before redirect matching.

## Summary

| Total findings | Auto-fixable (literal) | Manual review (dynamic) |
|----------------|------------------------|-------------------------|
| 23             | 23                     | 0                       |

## coder/coder/site

6 findings.

| File:Line                                                                                | Current path                                           | Redirect rule                                                           | Suggested fix                                                        | Dynamic? |
|------------------------------------------------------------------------------------------|--------------------------------------------------------|-------------------------------------------------------------------------|----------------------------------------------------------------------|----------|
| `site/src/components/Paywall/PaywallAIGovernance.tsx:47`                                 | `/ai-coder/ai-bridge`                                  | `/docs/ai-coder/ai-bridge/:path*` -> `/docs/ai-coder/ai-gateway/:path*` | `/ai-coder/ai-gateway`                                               | No       |
| `site/src/pages/AIBridgePage/AIBridgeHelpPopover.tsx:25`                                 | `/ai-coder/ai-bridge`                                  | `/docs/ai-coder/ai-bridge/:path*` -> `/docs/ai-coder/ai-gateway/:path*` | `/ai-coder/ai-gateway`                                               | No       |
| `site/src/pages/AIBridgePage/AIBridgeSessionsLayout.tsx:25`                              | `/ai-coder/ai-bridge/audit`                            | `/docs/ai-coder/ai-bridge/:path*` -> `/docs/ai-coder/ai-gateway/:path*` | `/ai-coder/ai-gateway/audit`                                         | No       |
| `site/src/pages/AIBridgePage/AIBridgeSetupAlert.tsx:15`                                  | `/ai-coder/ai-bridge`                                  | `/docs/ai-coder/ai-bridge/:path*` -> `/docs/ai-coder/ai-gateway/:path*` | `/ai-coder/ai-gateway`                                               | No       |
| `site/src/pages/AIBridgePage/SessionThreadsPage/SessionTimeline/SessionTimeline.tsx:321` | `/ai-coder/ai-bridge/audit#human-vs-agent-attribution` | `/docs/ai-coder/ai-bridge/:path*` -> `/docs/ai-coder/ai-gateway/:path*` | `/ai-coder/ai-gateway/audit` (preserve `#anchor` from current value) | No       |
| `site/src/pages/TemplatesPage/TemplatesFilter.tsx:63`                                    | `/templates#template-filtering`                        | `/docs/templates` -> `/docs/admin/templates`                            | `/admin/templates` (preserve `#anchor` from current value)           | No       |

## coder/coder.com/src

17 findings.

| File:Line                                       | Current path                                   | Redirect rule                                                                                    | Suggested fix                                                                                     | Dynamic? |
|-------------------------------------------------|------------------------------------------------|--------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|----------|
| `src/components/organisms/PricingTable.tsx:87`  | `/docs/workspaces`                             | `/docs/workspaces` -> `/docs/user-guides/workspace-management`                                   | `https://coder.com/docs/user-guides/workspace-management`                                         | No       |
| `src/components/organisms/PricingTable.tsx:123` | `/docs/ides/web-ides`                          | `/docs/ides/web-ides` -> `/docs/user-guides/workspace-access#other-web-ides`                     | `https://coder.com/docs/user-guides/workspace-access#other-web-ides`                              | No       |
| `src/components/organisms/PricingTable.tsx:135` | `/docs/ides`                                   | `/docs/ides` -> `/docs/user-guides/workspace-access`                                             | `https://coder.com/docs/user-guides/workspace-access`                                             | No       |
| `src/components/organisms/PricingTable.tsx:147` | `/docs/ides/vscode-extensions`                 | `/docs/ides/vscode-extensions` -> `/docs/user-guides/workspace-access/vscode#vs-code-extensions` | `https://coder.com/docs/user-guides/workspace-access/vscode#vs-code-extensions`                   | No       |
| `src/components/organisms/PricingTable.tsx:233` | `/docs/templates`                              | `/docs/templates` -> `/docs/admin/templates`                                                     | `https://coder.com/docs/admin/templates`                                                          | No       |
| `src/components/organisms/PricingTable.tsx:257` | `/docs/admin/prometheus`                       | `/docs/admin/prometheus` -> `/docs/admin/integrations/prometheus`                                | `https://coder.com/docs/admin/integrations/prometheus`                                            | No       |
| `src/components/organisms/PricingTable.tsx:325` | `/docs/admin/appearance#announcement-banners`  | `/docs/admin/appearance` -> `/docs/admin/setup/appearance`                                       | `https://coder.com/docs/admin/setup/appearance` (preserve `#anchor` from current value)           | No       |
| `src/components/organisms/PricingTable.tsx:337` | `/docs/workspaces#user-quiet-hours-enterprise` | `/docs/workspaces` -> `/docs/user-guides/workspace-management`                                   | `https://coder.com/docs/user-guides/workspace-management` (preserve `#anchor` from current value) | No       |
| `src/components/organisms/PricingTable.tsx:349` | `/docs/admin/appearance`                       | `/docs/admin/appearance` -> `/docs/admin/setup/appearance`                                       | `https://coder.com/docs/admin/setup/appearance`                                                   | No       |
| `src/components/organisms/PricingTable.tsx:390` | `/docs/admin/groups`                           | `/docs/admin/groups` -> `/docs/admin/users/groups-roles`                                         | `https://coder.com/docs/admin/users/groups-roles`                                                 | No       |
| `src/components/organisms/PricingTable.tsx:468` | `/docs/admin/quotas`                           | `/docs/admin/quotas` -> `/docs/admin/users/quotas`                                               | `https://coder.com/docs/admin/users/quotas`                                                       | No       |
| `src/components/organisms/PricingTable.tsx:480` | `/docs/admin/quotas`                           | `/docs/admin/quotas` -> `/docs/admin/users/quotas`                                               | `https://coder.com/docs/admin/users/quotas`                                                       | No       |
| `src/data/mockSuccessStories.ts:529`            | `/docs/platforms/kubernetes`                   | `/docs/platforms/:slug(.*)` -> `/docs/install/cloud`                                             | `https://coder.com/docs/install/cloud`                                                            | No       |
| `src/data/mockSuccessStories.ts:529`            | `/docs/admin/rbac`                             | `/docs/admin/rbac` -> `/docs/admin/templates/template-permissions`                               | `https://coder.com/docs/admin/templates/template-permissions`                                     | No       |
| `src/data/mockSuccessStories.ts:529`            | `/docs/admin/audit-logs`                       | `/docs/admin/audit-logs` -> `/docs/admin/security/audit-logs`                                    | `https://coder.com/docs/admin/security/audit-logs`                                                | No       |
| `src/data/mockSuccessStories.ts:577`            | `/docs/templates/parameters`                   | `/docs/templates/parameters` -> `/docs/admin/templates/extending-templates/parameters`           | `https://coder.com/docs/admin/templates/extending-templates/parameters`                           | No       |
| `src/data/mockSuccessStories.ts:577`            | `/docs/templates/variables`                    | `/docs/templates/variables` -> `/docs/admin/templates/extending-templates/variables`             | `https://coder.com/docs/admin/templates/extending-templates/variables`                            | No       |

## Notes

* Dynamic findings have a `${...}` expression somewhere in the path. The suggested fix shows what the literal prefix should become; the developer must keep the dynamic suffix intact.
* Findings under `docs/.audit/` or `docs/CHANGELOG` paths are excluded by file discovery to avoid feedback loops on the audit itself.
* Re-run with `node site/scripts/audit_docs_paths.mjs` from the repo root.
