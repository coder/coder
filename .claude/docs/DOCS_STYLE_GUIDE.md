# Documentation Style Guide

This guide documents structure, research, and content patterns for documentation files in the `docs/` directory. It complements, and does not replace, the canonical content rules or the prose style guide.

> [!IMPORTANT]
> **What belongs in the docs (and what doesn't)** is governed by
> [`docs/.style/content-guidelines.md`](../../docs/.style/content-guidelines.md).
> Read that first. When this style guide conflicts with the content
> guidelines, the content guidelines govern.
>
> **For prose rules**, refer to the canonical Coder documentation style guide at [`docs/.style/style-guide/`](../../docs/.style/style-guide/README.md).
> Vale rules under `docs/.style/styles/Coder/` enforce those rules incrementally as each rule lands.
> This file remains authoritative for structure, research, and content patterns.

See [CONTRIBUTING.md](../../docs/about/contributing/CONTRIBUTING.md) for general contribution guidelines.

## Research Before Writing

Before documenting a feature:

1. **Research similar documentation** - Read recent documentation pages in `docs/` to understand writing style, structure, and conventions for your content type (admin guides, tutorials, reference docs, etc.)
2. **Read the code implementation** - Check backend endpoints, frontend components, database queries
3. **Verify permissions model** - Look up RBAC actions in `coderd/rbac/` (e.g., `view_insights` for Template Insights)
4. **Check UI thresholds and defaults** - Review frontend code for color thresholds, time intervals, display logic
5. **Cross-reference with tests** - Test files document expected behavior and edge cases
6. **Verify API endpoints** - Check `coderd/coderd.go` for route registration

### Code Verification Checklist

When documenting features, always verify these implementation details:

- Read handler implementation in `coderd/`
- Check permission requirements in `coderd/rbac/`
- Review frontend components in `site/src/pages/` or `site/src/modules/`
- Verify display thresholds and intervals (e.g., color codes, time defaults)
- Confirm API endpoint paths and parameters
- Check for server flags in serpent configuration

## Document Structure

### Title and Introduction Pattern

**H1 heading**: Single clear title without prefix

```markdown
# Template Insights
```

**Introduction**: 1-2 sentences describing what the feature does, concise and actionable

```markdown
Template Insights provides detailed analytics and usage metrics for your Coder templates.
```

### Premium Feature Callout

For Premium-only features, add `(Premium)` suffix to the H1 heading. The documentation system automatically links these to premium pricing information. You should also add a premium badge in the `docs/manifest.json` file with `"state": ["premium"]`.

```markdown
# Template Insights (Premium)
```

### Overview Section Pattern

Common pattern after introduction:

```markdown
## Overview

Template Insights offers visibility into:

- **Active Users**: Track the number of users actively using workspaces
- **Application Usage**: See which applications users are accessing
```

Use bold labels for capabilities, provides high-level understanding before details.

## Image Usage

### Placement and Format

**Place images after descriptive text**, then add caption:

```markdown
![Template Insights page](../../images/admin/templates/template-insights.png)

<small>Template Insights showing weekly active users and connection latency metrics.</small>
```

- Image format: `![Descriptive alt text](../../path/to/image.png)`
- Caption: Use `<small>` tag below images
- Alt text: Describe what's shown, not just repeat heading

### Screenshot policy

Screenshots are governed by the canonical content guidelines. See
[Screenshots, used wisely](../../docs/.style/content-guidelines.md#what-belongs-in-the-docs)
in `docs/.style/content-guidelines.md`. The short version:

- Include a screenshot only when the topic would be confusing without
  the visual aid.
- No PHI or PII.
- No internal secrets leaked without obfuscation.
- Capture the minimally necessary surface area.
- Alt text is always required and must explain the screenshot's
  purpose for accessibility.

Do not structure sections around screenshots, and do not insert
placeholders for missing screenshots. Those older patterns are
superseded by the canonical content guidelines.

## Content Organization

### Section Hierarchy

1. **H2 (##)**: Major sections - "Overview", "Accessing [Feature]", "Use Cases"
2. **H3 (###)**: Subsections within major sections
3. **H4 (####)**: Rare, only for deeply nested content

### Common Section Patterns

- **Accessing [Feature]**: How to navigate to/use the feature
- **Use Cases**: Practical applications
- **Permissions**: Access control information
- **API Access**: Programmatic access details
- **Related Documentation**: Links to related content

### Lists and Callouts

- **Unordered lists**: Non-sequential items, features, capabilities
- **Ordered lists**: Step-by-step instructions
- **Tables**: Comparing options, showing permissions, listing parameters
- **Callouts**:
  - `> [!NOTE]` for additional information
  - `> [!WARNING]` for important warnings
  - `> [!TIP]` for helpful tips
- **Tabs**: Use tabs for presenting related but parallel content, such as different installation methods or platform-specific instructions. Tabs work well when readers need to choose one path that applies to their specific situation.

## Writing Style

### Tone and Voice

- **Direct and concise**: Avoid unnecessary words
- **Active voice**: "Template Insights tracks users" not "Users are tracked"
- **Present tense**: "The chart displays..." not "The chart will display..."
- **Second person**: "You can view..." for instructions

### Terminology

- **Consistent terms**: Use same term throughout (e.g., "workspace" not "workspace environment")
- **Bold for UI elements**: "Navigate to the **Templates** page"
- **Code formatting**: Use backticks for commands, file paths, code
  - Inline: `` `coder server` ``
  - Blocks: Use triple backticks with language identifier

### Punctuation

- Do not use emdash (U+2014), endash (U+2013), or ` -- ` as punctuation
  in code, comments, string literals, or documentation. Use commas,
  semicolons, or periods instead. Restructure the sentence if needed.
  For numeric ranges, use a plain hyphen (e.g., `0-100`).

### Instructions

- **Numbered lists** for sequential steps
- **Start with verb**: "Navigate to", "Click", "Select", "Run"
- **Be specific**: Include exact button/menu names in bold

## Code Examples

### Command Examples

````markdown
```sh
coder server --disable-template-insights
```
````

### Environment Variables

````markdown
```sh
CODER_DISABLE_TEMPLATE_INSIGHTS=true
```
````

### Code Comments

- Keep minimal
- Explain non-obvious parameters
- Use `# Comment` for shell, `// Comment` for other languages

## Links and References

### Internal Links

Use relative paths from current file location:

- `[Template Permissions](./template-permissions.md)`
- `[API documentation](../../reference/api/insights.md)`

For cross-linking to Coder registry templates or other external Coder resources, reference the appropriate registry URLs.

### Cross-References

- Link to related documentation at the end
- Use descriptive text: "Learn about [template access control](./template-permissions.md)"
- Not just: "[Click here](./template-permissions.md)"

### API References

Link to specific endpoints:

```markdown
- `/api/v2/insights/templates` - Template usage metrics
```

## Accuracy Standards

### Specific Numbers Matter

Document exact values from code:

- **Thresholds**: "green < 150ms, yellow 150-300ms, red ≥300ms"
- **Time intervals**: "daily for templates < 5 weeks old, weekly for 5+ weeks"
- **Counts and limits**: Use precise numbers, not approximations

### Permission Actions

- Use exact RBAC action names from code (e.g., `view_insights` not "view insights")
- Reference permission system correctly (`template:view_insights` scope)
- Specify which roles have permissions by default

### API Endpoints

- Use full, correct paths (e.g., `/api/v2/insights/templates` not `/insights/templates`)
- Link to generated API documentation in `docs/reference/api/`

## Documentation Manifest

**CRITICAL**: All documentation pages must be added to `docs/manifest.json` to appear in navigation. Read the manifest file to understand the structure and find the appropriate section for your documentation. Place new pages in logical sections matching the existing hierarchy.

## Documentation lands with the change

This rule lives in the canonical content guidelines. See
[Documentation lands with the change](../../docs/.style/content-guidelines.md#documentation-lands-with-the-change)
in `docs/.style/content-guidelines.md` for the rule, the definition of
"user-facing," the three corollaries, and the experiments-versus-feature-stages
distinction.

## Special Sections

### Prerequisites

- Bullet or numbered list
- Include version requirements, dependencies, permissions

## Sections that don't belong

### Troubleshooting

Troubleshooting and failure-mode content routes to the Support
knowledge base (Pylon), not the docs. Support is the primary owner;
Docs is secondary owner where needed. See the
[routing table](../../docs/.style/content-guidelines.md#routing-table)
in the canonical content guidelines.

Don't add a Troubleshooting section to a docs page. If a page would
benefit from troubleshooting context, surface it via the embedded
Pylon KB widget when that work lands; until then, link out to the
relevant Pylon article from the page body.

## Formatting and Linting

**Always run these commands before submitting documentation:**

```sh
make fmt/markdown   # Format markdown tables and content
make lint/markdown  # Lint and fix markdown issues
```

These ensure consistent formatting and catch common documentation errors.

## Formatting Conventions

### Text Formatting

- **Bold** (`**text**`): UI elements, important concepts, labels
- *Italic* (`*text*`): Rare, mainly for emphasis
- `Code` (`` `text` ``): Commands, file paths, parameter names

### Tables

- Use for comparing options, listing parameters, showing permissions
- Left-align text, right-align numbers
- Keep simple - avoid nested formatting when possible

### Code Blocks

- **Always specify language**: `` ```sh ``, `` ```yaml ``, `` ```go ``
- Include comments for complex examples
- Keep minimal - show only relevant configuration

## Document Length

- **Comprehensive but scannable**: Cover all aspects but use clear headings
- **Break up long sections**: Use H3 subheadings for logical chunks
- **Visual hierarchy**: Images and code blocks break up text

## Auto-Generated Content

Some content is auto-generated with comments:

```markdown
<!-- Code generated by 'make docs/...' DO NOT EDIT -->
```

Don't manually edit auto-generated sections.

## URL Redirects

When renaming or moving documentation pages, redirects must be added to prevent broken links.

**Important**: Redirects are NOT configured in this repository. The coder.com website runs on Vercel with Next.js and reads redirects from a separate repository:

- **Redirect configuration**: https://github.com/coder/coder.com/blob/master/redirects.json
- **Do NOT create** a `docs/_redirects` file - this format (used by Netlify/Cloudflare Pages) is not processed by coder.com

When you rename or move a doc page, create a PR in coder/coder.com to add the redirect.

## Key Principles

1. **Research first** - Verify against actual code implementation
2. **Be precise** - Use exact numbers, permission names, API paths
3. **Visual structure** - Organize around screenshots when available
4. **Link everything** - Related docs, API endpoints, CLI references
5. **Manifest inclusion** - Add to manifest.json for navigation
6. **Add redirects** - When moving/renaming pages, add redirects in coder/coder.com repo
