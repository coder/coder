import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		expect(dialog).toBeInTheDocument();
		expect(
			within(dialog).getByText(/This is some pasted text content\./i),
		).toBeInTheDocument();
	},
};

export const LongContent: Story = {
	args: {
		content: Array(100)
			.fill(
				"This is a line of pasted text that demonstrates how the dialog handles very long content.",
			)
			.join("\n"),
		onClose: () => {},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const content = within(dialog).getByText(
			/This is a line of pasted text that demonstrates how the dialog handles very long content\./i,
		);
		expect(content).toBeInTheDocument();
		expect(content.parentElement).toHaveClass("overflow-auto");
	},
};

export const NoFileName: Story = {
	args: {
		content: "Some pasted content without a filename.",
		onClose: () => {},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		expect(within(dialog).getByText("Pasted text")).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		// The heading should render as a real <h1>, not raw "# Auth split…".
		const heading = await within(dialog).findByRole("heading", {
			name: /Auth split runbook/i,
			level: 1,
		});
		expect(heading).toBeInTheDocument();
		// Inline link from the markdown should be a real anchor.
		const link = within(dialog).getByRole("link", {
			name: /the SDK types/i,
		});
		expect(link).toHaveAttribute("href", "https://example.com/sdk");
		// The verbatim "# " heading prefix must not appear as text. That
		// would mean we fell back to the plain <pre> renderer.
		expect(dialog.textContent ?? "").not.toContain("# Auth split runbook");
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const heading = await within(dialog).findByRole("heading", {
			name: /Auth split runbook/i,
			level: 1,
		});
		expect(heading).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		await waitFor(() => {
			// The markdown renderer schedules updates via useTransition,
			// so wait for the inline formatting nodes to appear before
			// asserting on them. Streamdown renders bold as a styled
			// <span data-streamdown="strong"> rather than a literal
			// <strong> element.
			const strong = dialog.querySelector('[data-streamdown="strong"]');
			expect(strong?.textContent).toBe("bold");
		});
		const em = dialog.querySelector("em");
		expect(em?.textContent).toBe("italic");
		// Inline code should render in a <code> element.
		const code = dialog.querySelector("code");
		expect(code?.textContent).toBe("code");
		// Raw markdown markers should not be visible as text.
		expect(dialog.textContent ?? "").not.toContain("**bold**");
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const pre = dialog.querySelector("pre");
		expect(pre).not.toBeNull();
		expect(pre?.textContent).toContain("function add(a, b)");
	},
};
