import { preloadHighlighter } from "@pierre/diffs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
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

const sampleFileCode = `package auth

import "errors"

func ValidateToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}`;

const sampleFileMarkdown = `
\`\`\`go
${sampleFileCode}
\`\`\`
`;

const mockClipboardWrite = () => {
	spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
};

const meta: Meta<typeof Response> = {
	title: "pages/AgentsPage/ChatElements/Response",
	component: Response,
	args: {
		children: sampleMarkdown,
	},
	// Without the app's worker pool a cold in-page highlighter loses its
	// first render under StrictMode, so code blocks that mount after the
	// initial story render (such as the Mermaid error fallback) stay
	// blank. Warming the themes first makes that render synchronous.
	loaders: [
		async () => {
			await preloadHighlighter({
				themes: ["github-dark-high-contrast", "github-light"],
				langs: [],
			});
		},
	],
};

export default meta;
type Story = StoryObj<typeof Response>;

export const MarkdownAndLinks: Story = {};

export const FencedFileBlock: Story = {
	args: {
		children: sampleFileMarkdown,
	},
	beforeEach: mockClipboardWrite,
	// Clicks the hover-only copy button so the capture shows the
	// copied confirmation state. Behavior is covered in Response.test.tsx.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const copyButton = await canvas.findByRole("button", {
			name: "Copy code",
		});
		await userEvent.click(copyButton);
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
	beforeEach: mockClipboardWrite,
	// Behavior is covered in Response.test.tsx.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const copyButton = await canvas.findByRole("button", {
			name: "Copy code",
		});
		await userEvent.click(copyButton);
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

const mermaidFlowchart = [
	"```mermaid",
	"flowchart TB",
	'  subgraph Leadership["Product leadership"]',
	"    BP[VP Product<br/>strategy, themes, PRD sign-off]",
	"    BG[Staff PM<br/>owns PDLC, roadmap health]",
	"    BP --> BG",
	"  end",
	'  subgraph Pods["Thematic pods"]',
	"    direction LR",
	'    A["Coder Agents"]',
	'    B["AI Governance"]',
	'    C["Enterprise Experience"]',
	"  end",
	'  subgraph Linear["Linear teams"]',
	"    L1[CODAGT]; L2[AIGOV]; L3[ENT]",
	"  end",
	"  Leadership --> Pods",
	"  A --> L1; B --> L2; C --> L3",
	"```",
	"",
].join("\n");

const mermaidSequence = [
	"The agent talks to the workspace like this:",
	"",
	"```mermaid",
	"sequenceDiagram",
	"  participant U as User",
	"  participant C as coderd",
	"  participant W as Workspace agent",
	"  U->>C: POST /api/v2/chats",
	"  C->>W: Start task",
	"  W-->>C: Stream tool output",
	"  C-->>U: Render response",
	"```",
	"",
	"Each hop is authenticated separately.",
].join("\n");

const waitForDiagram = (canvasElement: HTMLElement) =>
	within(canvasElement).findByRole(
		"button",
		{ name: "View diagram full size" },
		{ timeout: 10_000 },
	);

// Mermaid renders asynchronously after its chunk loads, so these
// stories wait for the rendered diagram before the capture.
export const MermaidFlowchart: Story = {
	args: {
		children: mermaidFlowchart,
	},
	play: async ({ canvasElement }) => {
		await waitForDiagram(canvasElement);
	},
};

export const MermaidFlowchartLight: Story = {
	args: {
		children: mermaidFlowchart,
	},
	globals: {
		theme: "light",
	},
	play: async ({ canvasElement }) => {
		await waitForDiagram(canvasElement);
	},
};

export const MermaidSequenceInProse: Story = {
	args: {
		children: mermaidSequence,
	},
	play: async ({ canvasElement }) => {
		await waitForDiagram(canvasElement);
	},
};

// Clicking a rendered diagram opens it at natural size in a lightbox,
// the same affordance chat images have.
export const MermaidLightbox: Story = {
	args: {
		children: mermaidFlowchart,
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(await waitForDiagram(canvasElement));
		await within(document.body).findByRole("dialog", {
			name: "Diagram preview",
		});
	},
};

// A parse error shows the Mermaid message and keeps the source
// visible as a regular code block underneath.
export const MermaidSyntaxError: Story = {
	args: {
		children: [
			"```mermaid",
			'flowchart TBsubgraph Leadership["Product leadership"]',
			"  BP --> BG",
			"end",
			"```",
		].join("\n"),
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("alert", {}, { timeout: 10_000 });
	},
};

// While the fence is still open the diagram is not rendered, so the
// viewer sees a stable placeholder instead of a stream of parse
// errors from half-written source.
export const StreamingMermaidFence: Story = {
	args: {
		children: "```mermaid\nflowchart LR\n  A[Start] --> B[Sec",
		streaming: true,
	},
};
