# Documentation Style Guide

This guide documents documentation patterns observed in the Coder repository, based on analysis of existing admin guides, tutorials, and reference documentation. This is specifically for documentation files in the `docs/` directory - see [CONTRIBUTING.md](../../docs/about/contributing/CONTRIBUTING.md) for general contribution guidelines.

## Research Before Writing

Before documenting a feature, verify implementation details against the actual codebase:

1. **Read the code implementation** - Check backend endpoints, frontend components, database queries
2. **Verify permissions model** - Look up RBAC actions in `coderd/rbac/` (e.g., `view_insights` for Template Insights)
3. **Check UI thresholds and defaults** - Review frontend code for color thresholds, time intervals, display logic
4. **Cross-reference with tests** - Test files document expected behavior and edge cases
5. **Verify API endpoints** - Check `coderd/coderd.go` for route registration

### Code Verification Checklist

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

Place immediately after title if feature requires Premium license:

```markdown
> [!NOTE]
> Feature name is a Premium feature.
> [Learn more](https://coder.com/pricing#compare-plans).
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

### Image-Driven Documentation

When you have multiple screenshots showing different aspects of a feature:

1. **Structure sections around images** - Each major screenshot gets its own section
2. **Describe what's visible** - Reference specific UI elements, data values shown in the screenshot
3. **Flow naturally** - Let screenshots guide the reader through the feature

**Example**: Template Insights documentation has 3 screenshots that define the 3 main content sections.

### Screenshot Guidelines

- Illustrate features being discussed in preceding text
- Show actual UI/data, not abstract concepts
- Reference specific values shown when explaining features
- Organize documentation around key screenshots

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

### Instructions

- **Numbered lists** for sequential steps
- **Start with verb**: "Navigate to", "Click", "Select", "Run"
- **Be specific**: Include exact button/menu names in bold

## Code Examples

### Command Examples

```markdown
```sh
coder server --disable-template-insights
```
```

### Environment Variables

```markdown
```sh
CODER_DISABLE_TEMPLATE_INSIGHTS=true
```
```

### Code Comments

- Keep minimal
- Explain non-obvious parameters
- Use `# Comment` for shell, `// Comment` for other languages

## Links and References

### Internal Links

Use relative paths from current file location:

- `[Template Permissions](./template-permissions.md)`
- `[API documentation](../../reference/api/insights.md)`

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

**All documentation pages must be added to `docs/manifest.json`** to appear in navigation:

```json
{
  "title": "Template Insights",
  "description": "Monitor template usage and application metrics",
  "path": "./admin/templates/template-insights.md"
}
```

- **title**: Display name in navigation
- **description**: Brief summary (used in some UI contexts)
- **path**: Relative path from `docs/` directory
- **state** (optional): `["premium"]` or `["beta"]` for feature badges

Place new pages in logical sections matching existing hierarchy in manifest.json.

## Proactive Documentation

When documenting features that depend on upcoming PRs:

1. **Reference the PR explicitly** - Mention PR number and what it adds
2. **Document the feature anyway** - Write as if feature exists
3. **Link to auto-generated docs** - Point to CLI reference sections that will be created
4. **Update PR description** - Note documentation is included proactively

**Example**: Template Insights docs include `--disable-template-insights` flag from PR #20940 before it merged, with link to `../../reference/cli/server.md#--disable-template-insights` that will exist when the PR lands.

## Special Sections

### Troubleshooting

- **H3 subheadings** for each issue
- Format: Issue description followed by solution steps

### Prerequisites

- Bullet or numbered list
- Include version requirements, dependencies, permissions

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

## Key Principles

1. **Research first** - Verify against actual code implementation
2. **Be precise** - Use exact numbers, permission names, API paths
3. **Visual structure** - Organize around screenshots when available
4. **Link everything** - Related docs, API endpoints, CLI references
5. **Manifest inclusion** - Add to manifest.json for navigation
