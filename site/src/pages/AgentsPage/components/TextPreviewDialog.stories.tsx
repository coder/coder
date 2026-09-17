import type { Meta, StoryObj } from "@storybook/react-vite";
import { TextPreviewDialog } from "./TextPreviewDialog";

const meta: Meta<typeof TextPreviewDialog> = {
	title: "pages/AgentsPage/TextPreviewDialog",
	component: TextPreviewDialog,
};

export default meta;
type Story = StoryObj<typeof TextPreviewDialog>;

export const Default: Story = {
	args: {
		content:
			"This is some pasted text content.\nIt has multiple lines.\nAnd should be displayed in a readable format.",
		onClose: () => {},
	},
};

export const LongContent: Story = {
	args: {
		content: new Array(100)
			.fill(
				"This is a line of pasted text that demonstrates how the dialog handles very long content.",
			)
			.join("\n"),
		onClose: () => {},
	},
};

export const NoFileName: Story = {
	args: {
		content: "Some pasted content without a filename.",
		onClose: () => {},
	},
};

const sampleMarkdown = `# Auth split runbook

This document captures the rollout plan for the upcoming auth split.

## Goals

1. Move OAuth2 endpoints under \`coderd/oauth2/\`.
2. Keep external auth providers behind their existing routes.
3. Avoid downtime for in-flight tokens.

> Reviewers should pay close attention to the migration order, since
> dropping the legacy table before backfilling will lose tokens.

## Checklist

- [x] Draft the migration in \`coderd/database/migrations/\`.
- [x] Update [the SDK types](https://example.com/sdk).
- [ ] Coordinate with the deployments team.

## Rollout window

| Phase | Date       | Owner   |
| ----- | ---------- | ------- |
| Beta  | 2025-07-15 | @kyle   |
| GA    | 2025-08-01 | @ammar  |

## Sample query

\`\`\`sql
SELECT id, user_id, provider
FROM oauth2_tokens
WHERE provider = 'github';
\`\`\`

Inline guidance: prefer \`AsSystemRestricted\` over \`AsSystem\`.
`;

/** Markdown attachments should render with the same formatter we use for
 * chat messages, so headings, lists, tables, and fenced code all look
 * native instead of appearing as a raw monospaced dump. */
export const MarkdownByExtension: Story = {
	args: {
		content: sampleMarkdown,
		fileName: "AUTH_SPLIT.md",
		onClose: () => {},
	},
};

/** Equivalent to MarkdownByExtension but driven entirely by the explicit
 * media type so we cover the case where a file lacks a `.md` suffix but
 * the upload pipeline still tagged it as `text/markdown`. */
export const MarkdownByMediaType: Story = {
	args: {
		content: sampleMarkdown,
		fileName: "runbook",
		mediaType: "text/markdown",
		onClose: () => {},
	},
};

/** When the file looks like markdown but the body is just plain prose, the
 * Markdown renderer should still produce a clean paragraph rather than a
 * monospaced block. */
export const MarkdownProseOnly: Story = {
	args: {
		content:
			"Just a short paragraph of prose with **bold** and _italic_ runs and an inline `code` token.",
		fileName: "notes.md",
		onClose: () => {},
	},
};

/** Plain `.txt` files should keep the existing monospaced rendering so we
 * don't regress the original code-style preview. */
export const PlainTextStaysMonospaced: Story = {
	args: {
		content: "function add(a, b) {\n  return a + b;\n}\n",
		fileName: "snippet.txt",
		onClose: () => {},
	},
};
