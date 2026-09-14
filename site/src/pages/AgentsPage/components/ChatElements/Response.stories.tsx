import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Response } from "./Response";

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
	title: "pages/AgentsPage/ChatElements/Response",
	component: Response,
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

const singleLineCodeBlockMarkdown = `
\`\`\`
07c3697 feat: update agent skills
\`\`\`
`;

export const SingleLineFencedBlock: Story = {
	args: {
		children: singleLineCodeBlockMarkdown,
	},
};

const longLineCodeBlockMarkdown = [
	"```ts",
	'const config = { apiUrl: "https://coder.example.com/api/v2/workspaces", token: "abcdefghijklmnopqrstuvwxyz0123456789_ABCDEFGHIJKLMNOPQRSTUVWXYZ_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", retries: 5 };',
	"```",
	"",
].join("\n");

export const LongLineFencedBlock: Story = {
	args: {
		children: longLineCodeBlockMarkdown,
	},
	play: async ({ canvasElement }) => {
		// The fenced block renders asynchronously inside the FileViewer
		// shadow root, so retry until a horizontally scrollable viewport
		// exists, then scroll it so the capture shows the scrolled state.
		const viewport = await waitFor(() => {
			const found = [
				...canvasElement.querySelectorAll<HTMLElement>(
					"[data-radix-scroll-area-viewport]",
				),
			].find((v) => v.scrollWidth > v.clientWidth);
			if (!found) {
				throw new Error("Expected a horizontally scrollable viewport.");
			}
			return found;
		});
		viewport.scrollLeft = 200;
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
};

// A 1x1 transparent PNG. Streamdown's sanitize plugin strips data:
// image sources before our img component sees them, so these render
// as nothing: inert, and never a network request.
const dataImagePNG =
	"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==";

const externalImageURL = "https://external-image-host.invalid/image.png";

// Verifies the IP-leak fix for Cure53 CDM-02-006: externally hosted
// markdown images must not be fetched when a chat is rendered. The
// viewer gets a consent placeholder and the <img> element only
// appears after clicking it.
export const ExternalImageConsentGate: Story = {
	args: {
		children: `Before\n\n![diagram](${externalImageURL})\n\nAfter`,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The placeholder must render instead of the image.
		const loadButton = await canvas.findByRole("button", {
			name: /load external image from external-image-host\.invalid/i,
		});
		expect(loadButton).toBeInTheDocument();

		// No <img> in the document may point at the external host.
		expect(canvasElement.querySelector("img")).toBeNull();

		// Clicking the placeholder opts in and renders the image.
		await userEvent.click(loadButton);
		await waitFor(() => {
			const img = canvasElement.querySelector("img");
			expect(img).not.toBeNull();
			expect(img?.getAttribute("src")).toBe(externalImageURL);
		});
	},
};

// data: image sources are stripped by the sanitize plugin, so they
// render as nothing: no <img>, no consent gate, no request.
export const DataImageStrippedBySanitizer: Story = {
	args: {
		children: `Before\n\n![inline](${dataImagePNG})\n\nAfter`,
	},
};

// Deployment-relative images (for example emoji or uploaded icons)
// are same-origin, so they render immediately without a consent gate.
export const RelativeImageRendersImmediately: Story = {
	args: {
		children: "![emoji](/emojis/1f4bb.png)",
	},
};

// Verifies that streaming mode closes incomplete inline markdown via
// remend so the user never sees raw syntax during the reveal animation.
export const StreamingInlineMarkdown: Story = {
	args: {
		children: "This is **bold text that has not been close",
		streaming: true,
	},
};

// The streaming external-image consent gate: the placeholder renders
// instead of an <img>, so no external request fires mid-stream.
export const StreamingExternalImageConsentGate: Story = {
	args: {
		children: `![diagram](${externalImageURL})`,
		streaming: true,
	},
};

// Verifies that an incomplete fenced code block in streaming mode
// renders as code rather than showing raw backticks.
export const StreamingCodeFence: Story = {
	args: {
		children: "```ts\nconst x = 1",
		streaming: true,
	},
};
