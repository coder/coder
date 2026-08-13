import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { rewriteLocalhostURL } from "#/utils/portForward";
import {
	type ChatUrlTransform,
	ChatUrlTransformContext,
} from "../../context/ChatUrlTransformContext";
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
	play: async ({ canvasElement }) => {
		await expectCodeBlock(canvasElement, /func ValidateToken/, {
			highlighted: true,
		});
	},
};

const singleLineCodeBlockMarkdown = `
\`\`\`
07c3697 feat: update agent skills
\`\`\`
`;

const findCodeBlockHost = async (canvasElement: HTMLElement, text: RegExp) => {
	let host: HTMLElement | undefined;
	await waitFor(() => {
		const hosts = Array.from(
			canvasElement.querySelectorAll("diffs-container"),
		).filter(
			(element): element is HTMLElement => element instanceof HTMLElement,
		);
		host = hosts.find((element) => {
			text.lastIndex = 0;
			return text.test(element.shadowRoot?.textContent ?? "");
		});
		expect(host).toBeDefined();
	});

	if (!host) {
		throw new Error("Expected fenced code to render inside FileViewer.");
	}
	return host;
};

const expectCodeBlock = async (
	canvasElement: HTMLElement,
	text: RegExp,
	options: { highlighted?: boolean } = {},
) => {
	const host = await findCodeBlockHost(canvasElement, text);
	expect(host).toBeInTheDocument();
	expect(host.style.getPropertyValue("--diffs-font-size")).toBe("12px");
	expect(host.style.getPropertyValue("--diffs-line-height")).toBe("20px");

	expect(canvasElement.textContent ?? "").not.toContain("```");

	const shadowRoot = host.shadowRoot;
	if (!shadowRoot) {
		throw new Error("Expected FileViewer to render code in its shadow root.");
	}

	if (options.highlighted) {
		await waitFor(() => {
			const token = shadowRoot.querySelector("span[style*='color']");
			expect(token).toBeInTheDocument();
		});
	}

	return host;
};

export const SingleLineFencedBlock: Story = {
	args: {
		children: singleLineCodeBlockMarkdown,
	},
	play: async ({ canvasElement }) => {
		await expectCodeBlock(canvasElement, /07c3697 feat/);
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
		await expectCodeBlock(canvasElement, /apiUrl/);
		const viewport = [
			...canvasElement.querySelectorAll<HTMLElement>(
				"[data-radix-scroll-area-viewport]",
			),
		].find((v) => v.scrollWidth > v.clientWidth);
		if (!viewport) {
			throw new Error("Expected a horizontally scrollable viewport.");
		}
		viewport.scrollLeft = 200;
		await waitFor(() => expect(viewport.scrollLeft).toBeGreaterThan(0));
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

const chatUrlTransform = (url: string) =>
	rewriteLocalhostURL(url, "*.proxy.example.com", "main", "my-ws", "alice");

export const LocalhostLinkFromChatContext: Story = {
	decorators: [
		(Story) => (
			<ChatUrlTransformContext value={chatUrlTransform}>
				<Story />
			</ChatUrlTransformContext>
		),
	],
	args: {
		children:
			"Open [the preview](http://localhost:3000/apps?tab=1) or [the docs](https://coder.com/docs).",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const preview = await canvas.findByRole("link", { name: "the preview" });
		expect(preview).toHaveAttribute(
			"href",
			"http://3000--main--my-ws--alice.proxy.example.com/apps?tab=1",
		);
		const docs = canvas.getByRole("link", { name: "the docs" });
		expect(docs).toHaveAttribute("href", "https://coder.com/docs");
	},
};

const TransformArrivesAfterRender: FC<{ children: string }> = ({
	children,
}) => {
	const [transform, setTransform] = useState<ChatUrlTransform | undefined>(
		undefined,
	);
	return (
		<ChatUrlTransformContext value={transform}>
			<button
				type="button"
				onClick={() => setTransform(() => chatUrlTransform)}
			>
				Load workspace data
			</button>
			<Response streaming>{children}</Response>
		</ChatUrlTransformContext>
	);
};

export const LocalhostLinkRewrittenMidStream: Story = {
	render: () => (
		<TransformArrivesAfterRender>
			Open [the preview](http://localhost:3000/apps?tab=1) now.
		</TransformArrivesAfterRender>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const preview = await canvas.findByRole("link", { name: "the preview" });
		expect(preview).toHaveAttribute("href", "http://localhost:3000/apps?tab=1");
		await userEvent.click(
			canvas.getByRole("button", { name: "Load workspace data" }),
		);
		await waitFor(() => {
			expect(canvas.getByRole("link", { name: "the preview" })).toHaveAttribute(
				"href",
				"http://3000--main--my-ws--alice.proxy.example.com/apps?tab=1",
			);
		});
	},
};

// data: image sources are stripped by the sanitize plugin, so they
// render as nothing: no <img>, no consent gate, no request.
export const DataImageStrippedBySanitizer: Story = {
	args: {
		children: `Before\n\n![inline](${dataImagePNG})\n\nAfter`,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("After");
		expect(canvasElement.querySelector("img")).toBeNull();
		expect(canvas.queryByRole("button")).toBeNull();
	},
};

// Deployment-relative images (for example emoji or uploaded icons)
// are same-origin, so they render immediately without a consent gate.
export const RelativeImageRendersImmediately: Story = {
	args: {
		children: "![emoji](/emojis/1f4bb.png)",
	},
	play: async ({ canvasElement }) => {
		await waitFor(() => {
			const img = canvasElement.querySelector("img");
			expect(img).not.toBeNull();
			expect(img?.getAttribute("src")).toBe("/emojis/1f4bb.png");
		});
		expect(within(canvasElement).queryByRole("button")).toBeNull();
	},
};

// The consent gate must also apply while streaming.
export const StreamingExternalImageConsentGate: Story = {
	args: {
		children: `![diagram](${externalImageURL})`,
		streaming: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const loadButton = await canvas.findByRole("button", {
			name: /load external image/i,
		});
		expect(loadButton).toBeInTheDocument();
		expect(canvasElement.querySelector("img")).toBeNull();
	},
};

const TransformSwitchesWorkspace: FC = () => {
	const [workspace, setWorkspace] = useState("ws-a");
	const transform = (url: string) =>
		rewriteLocalhostURL(url, "*.proxy.example.com", "main", workspace, "alice");
	return (
		<ChatUrlTransformContext value={transform}>
			<button type="button" onClick={() => setWorkspace("ws-b")}>
				Switch workspace
			</button>
			<Response>Open [the preview](http://localhost:3000/).</Response>
		</ChatUrlTransformContext>
	);
};

export const LocalhostLinkRetargetsOnWorkspaceChange: Story = {
	render: () => <TransformSwitchesWorkspace />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const preview = await canvas.findByRole("link", { name: "the preview" });
		expect(preview).toHaveAttribute(
			"href",
			"http://3000--main--ws-a--alice.proxy.example.com/",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Switch workspace" }),
		);
		await waitFor(() => {
			expect(canvas.getByRole("link", { name: "the preview" })).toHaveAttribute(
				"href",
				"http://3000--main--ws-b--alice.proxy.example.com/",
			);
		});
	},
};

export const LocalhostImageRewrittenFromChatContext: Story = {
	decorators: [
		(Story) => (
			<ChatUrlTransformContext value={chatUrlTransform}>
				<Story />
			</ChatUrlTransformContext>
		),
	],
	args: {
		children: "![preview shot](http://localhost:3000/screenshot.png)",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The rewritten source is cross-origin, so the external-image
		// consent gate applies to the transformed URL, not the raw
		// localhost one.
		const loadButton = await canvas.findByRole("button", {
			name: /load external image from 3000--main--my-ws--alice\.proxy\.example\.com/i,
		});
		await userEvent.click(loadButton);
		const image = await canvas.findByRole("img", { name: "preview shot" });
		expect(image).toHaveAttribute(
			"src",
			"http://3000--main--my-ws--alice.proxy.example.com/screenshot.png",
		);
	},
};

// Verifies that streaming mode closes incomplete inline markdown via
// remend so the user never sees raw syntax during the reveal animation.
export const StreamingInlineMarkdown: Story = {
	args: {
		children: "This is **bold text that has not been close",
		streaming: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// remend should close the unclosed ** so the text renders
		// as <strong>, not as a raw "**" literal.
		const el = await canvas.findByText(/bold text/);
		expect(el).toBeInTheDocument();
		// The raw double-asterisk should not appear as visible text.
		const bodyText = canvasElement.textContent ?? "";
		expect(bodyText).not.toContain("**");
	},
};

// Verifies that an incomplete fenced code block in streaming mode
// renders as code rather than showing raw backticks.
export const StreamingCodeFence: Story = {
	args: {
		children: "```ts\nconst x = 1",
		streaming: true,
	},
	play: async ({ canvasElement }) => {
		await expectCodeBlock(canvasElement, /const x = 1/, {
			highlighted: true,
		});

		// The raw triple-backtick should not appear as visible text.
		const bodyText = canvasElement.textContent ?? "";
		expect(bodyText).not.toContain("```");
	},
};
