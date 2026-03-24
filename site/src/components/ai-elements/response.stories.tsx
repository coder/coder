import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Response } from "./response";

const sampleMarkdown = `
## Plan update

I checked the auth flow and found two issues:

1. Missing provider fallback for unknown IDs.
2. Error text was not surfaced in the UI.

See [external auth docs](https://coder.com/docs) for expected behavior.

Inline command example: \`git fetch origin\`.

\`\`\`ts
export const ensureProviderLabel = (provider: string) => {
	return provider.trim() || "Git provider";
};
\`\`\`
`;

const sampleFileMarkdown = `
\`\`\`go
package auth

import "errors"

func ValidateToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}
\`\`\`
`;

const meta: Meta<typeof Response> = {
	title: "components/ai-elements/Response",
	component: Response,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		children: sampleMarkdown,
	},
};

export default meta;
type Story = StoryObj<typeof Response>;

export const MarkdownAndLinks: Story = {};

export const FencedFileBlock: Story = {
	args: {
		children: sampleFileMarkdown,
	},
};

export const MarkdownAndLinksLight: Story = {
	globals: {
		theme: "light",
	},
};

// Verifies that JSX-like syntax in LLM output is preserved as
// escaped text rather than being swallowed by the HTML pipeline.
const jsxProseMarkdown = `
\`getLineAnnotations\` depends on \`activeCommentBox\` which could shift.

<RemoteDiffPanel
  commentBox={commentBox}
  scrollToFile={scrollTarget}
  onScrollToFileComplete={handleScrollComplete}
/>

The props that might change on every \`RemoteDiffPanel\` re-render:
- \`isLoading\` only during refetch
- \`getLineAnnotations\` only when \`activeCommentBox\` changes
`;

export const JsxInProse: Story = {
	args: {
		children: jsxProseMarkdown,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// These strings live inside the <RemoteDiffPanel .../> JSX block.
		// Without the rehype-raw fix they are silently eaten by the
		// HTML sanitizer and never reach the DOM.
		// The tag name itself is the token most likely to be consumed
		// by HTML parsing, so assert it explicitly.
		const tagName = await canvas.findByText(/<RemoteDiffPanel/);
		expect(tagName).toBeInTheDocument();
		const marker = await canvas.findByText(/scrollToFile=\{scrollTarget\}/);
		expect(marker).toBeInTheDocument();
		const marker2 = await canvas.findByText(/commentBox=\{commentBox\}/);
		expect(marker2).toBeInTheDocument();
	},
};
