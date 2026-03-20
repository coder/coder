import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import { expect, fn, userEvent, within } from "storybook/test";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import type { ParsedMessageContent, ParsedMessageEntry } from "./types";

// 1×1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

const buildMessages = (messages: TypesGen.ChatMessage[]) =>
	parseMessagesWithMergedTools(messages);

const baseMessage = {
	chat_id: "story-chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

const defaultArgs: Omit<
	React.ComponentProps<typeof ConversationTimeline>,
	"parsedMessages"
> = {
	isEmpty: false,
	hasStreamOutput: false,
	streamState: null,
	streamTools: [],
	subagentTitles: new Map(),
	subagentStatusOverrides: new Map(),
	isAwaitingFirstStreamChunk: false,
};

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/AgentDetail/ConversationTimeline",
	component: ConversationTimeline,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6">
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof ConversationTimeline>;

const createParsedEntry = ({
	id,
	role,
	createdAt = baseMessage.created_at,
	content = [],
	parsed,
}: {
	id: number;
	role: TypesGen.ChatMessageRole;
	createdAt?: string;
	content?: readonly TypesGen.ChatMessagePart[];
	parsed?: Partial<ParsedMessageContent>;
}): ParsedMessageEntry => ({
	message: {
		...baseMessage,
		id,
		created_at: createdAt,
		role,
		content,
	},
	parsed: {
		markdown: "",
		reasoning: "",
		toolCalls: [],
		toolResults: [],
		tools: [],
		blocks: [],
		sources: [],
		...parsed,
	},
});

const makeTimestamp = (minuteOffset: number): string =>
	`2026-03-10T00:${String(minuteOffset).padStart(2, "0")}:00.000Z`;

const denseMarkdownText = `# Transcript rendering overview

## Formatting coverage

This story verifies **bold text**, _italic emphasis_, [external links](https://coder.com/docs), and inline code like \`parseMessagesWithMergedTools()\`.

### Ordered checklist

1. Parse the raw message parts.
2. Merge related tool calls and results.
3. Render the ordered transcript blocks.

### Unordered notes

- Tables rely on GFM support.
- Blockquotes should keep their inset styling.
- Horizontal rules separate dense sections cleanly.

#### Render block matrix

| Block type | Purpose | Example |
| --- | --- | --- |
| response | Assistant markdown | Final reply |
| thinking | Reasoning text | Planning step |
| tool | Tool card | read_file |

> Dense markdown fixtures help us catch typography and spacing regressions.

\`\`\`md
# Inline markdown sample
- bullet item
- \`inline snippet\`
\`\`\`

---

Final paragraph with **emphasis**, _style_, and a trailing [reference link](https://storybook.js.org/).`;

const longTypeScriptBlock = [
	"type Row = { id: string; label: string; enabled: boolean; description: string };",
	"",
	...Array.from({ length: 32 }, (_, index) => {
		const padded = String(index).padStart(2, "0");
		return `export const row${padded}: Row = { id: "row-${padded}", label: "Timeline item ${padded}", enabled: ${index % 2 === 0}, description: "Storybook coverage line ${padded}" };`;
	}),
	"",
	'const oversizedSummary = "This intentionally oversized line keeps going well past one hundred and twenty characters so horizontal overflow handling stays visible in Storybook snapshots.";',
].join("\n");

const longCodeBlocksText = `Here are several large snippets that exercise fenced code rendering.

\`\`\`go
package main

import "fmt"

func main() {
\tfmt.Println("conversation timeline")
}
\`\`\`

\`\`\`typescript
${longTypeScriptBlock}
\`\`\`

\`\`\`python
def summarize_events(events: list[str]) -> str:
    return ", ".join(event.strip() for event in events if event)

print(summarize_events(["tool called", "result merged", "ui rendered"]))
\`\`\`

\`\`\`sql
select message_id, role, created_at
from chat_messages
where chat_id = 'story-chat'
order by created_at asc;
\`\`\``;

const diffHeavyButtonPath = "site/src/components/Button.tsx";

const diffHeavyButtonBefore = [
	'import React from "react";',
	"",
	"interface ButtonProps {",
	"  label: string;",
	"  onClick: () => void;",
	'  variant?: "primary" | "secondary";',
	"  disabled?: boolean;",
	"}",
	"",
	"export const Button: React.FC<ButtonProps> = ({",
	"  label,",
	"  onClick,",
	'  variant = "primary",',
	"  disabled = false,",
	"}) => {",
	"  return (",
	"    <button",
	"      className={`btn btn-${variant}`}",
	"      onClick={onClick}",
	"      disabled={disabled}",
	"    >",
	"      {label}",
	"    </button>",
	"  );",
	"};",
].join("\n");

const diffHeavyButtonAfter = [
	'import React from "react";',
	"",
	"interface ButtonProps {",
	"  label: string;",
	"  onClick: () => void;",
	'  variant?: "primary" | "secondary";',
	'  size?: "sm" | "md" | "lg";',
	"  disabled?: boolean;",
	"}",
	"",
	"export const Button: React.FC<ButtonProps> = ({",
	"  label,",
	"  onClick,",
	'  variant = "primary",',
	'  size = "md",',
	"  disabled = false,",
	"}) => {",
	"  return (",
	"    <button",
	"      className={`btn btn-${variant} btn-${size}`}",
	"      onClick={onClick}",
	"      disabled={disabled}",
	"    >",
	"      {label}",
	"    </button>",
	"  );",
	"};",
].join("\n");

const diffHeavyOutputMessages: ParsedMessageEntry[] = [
	createParsedEntry({
		id: 200,
		role: "assistant",
		content: [
			{
				type: "text",
				text: "I'll update the Button component and create a new utility file.",
			},
		],
		parsed: {
			markdown:
				"I'll update the Button component and create a new utility file.",
			toolCalls: [
				{
					id: "tc_edit",
					name: "edit_files",
					args: {
						files: [
							{
								path: diffHeavyButtonPath,
								edits: [
									{
										search: diffHeavyButtonBefore,
										replace: diffHeavyButtonAfter,
									},
								],
							},
						],
					},
				},
				{
					id: "tc_write",
					name: "write_file",
					args: {
						path: "src/utils/format.ts",
						content:
							"export function formatDate(date: Date): string {\n  return date.toISOString().split('T')[0];\n}\n\nexport function formatCurrency(amount: number): string {\n  return new Intl.NumberFormat('en-US', {\n    style: 'currency',\n    currency: 'USD',\n  }).format(amount);\n}\n",
					},
				},
			],
			toolResults: [
				{
					id: "tc_edit",
					name: "edit_files",
					result: { output: "success" },
					isError: false,
				},
				{
					id: "tc_write",
					name: "write_file",
					result: {},
					isError: false,
				},
			],
			tools: [
				{
					id: "tc_edit",
					name: "edit_files",
					args: {
						files: [
							{
								path: diffHeavyButtonPath,
								edits: [
									{
										search: diffHeavyButtonBefore,
										replace: diffHeavyButtonAfter,
									},
								],
							},
						],
					},
					result: { output: "success" },
					isError: false,
					status: "completed",
				},
				{
					id: "tc_write",
					name: "write_file",
					args: {
						path: "src/utils/format.ts",
						content:
							"export function formatDate(date: Date): string {\n  return date.toISOString().split('T')[0];\n}\n\nexport function formatCurrency(amount: number): string {\n  return new Intl.NumberFormat('en-US', {\n    style: 'currency',\n    currency: 'USD',\n  }).format(amount);\n}\n",
					},
					result: {},
					isError: false,
					status: "completed",
				},
			],
			blocks: [
				{
					type: "response",
					text: "I'll update the Button component and create a new utility file.",
				},
				{ type: "tool", id: "tc_edit" },
				{ type: "tool", id: "tc_write" },
			],
		},
	}),
];

const reasoningAndToolStackMessages: ParsedMessageEntry[] = [
	createParsedEntry({
		id: 210,
		role: "assistant",
		content: [
			{
				type: "reasoning",
				text: "I need to understand the current file structure before making changes. Let me read the config file first, then edit it, and finally run the tests.",
			},
			{
				type: "text",
				text: "I'll read, edit, and test the configuration.",
			},
		],
		parsed: {
			markdown: "I'll read, edit, and test the configuration.",
			reasoning:
				"I need to understand the current file structure before making changes. Let me read the config file first, then edit it, and finally run the tests.",
			toolCalls: [
				{ id: "tc_r1", name: "read_file", args: { path: "tsconfig.json" } },
				{
					id: "tc_r2",
					name: "edit_files",
					args: {
						files: [
							{
								path: "tsconfig.json",
								edits: [
									{
										search: '"target": "es2020"',
										replace: '"target": "es2022"',
									},
								],
							},
						],
					},
				},
				{
					id: "tc_r3",
					name: "execute",
					args: { command: "npm run typecheck" },
				},
			],
			toolResults: [
				{
					id: "tc_r1",
					name: "read_file",
					result: { content: '{ "compilerOptions": { "target": "es2020" } }' },
					isError: false,
				},
				{
					id: "tc_r2",
					name: "edit_files",
					result: { output: "success" },
					isError: false,
				},
				{
					id: "tc_r3",
					name: "execute",
					result: { output: "✓ No type errors found" },
					isError: false,
				},
			],
			tools: [
				{
					id: "tc_r1",
					name: "read_file",
					args: { path: "tsconfig.json" },
					result: { content: '{ "compilerOptions": { "target": "es2020" } }' },
					isError: false,
					status: "completed",
				},
				{
					id: "tc_r2",
					name: "edit_files",
					args: {
						files: [
							{
								path: "tsconfig.json",
								edits: [
									{
										search: '"target": "es2020"',
										replace: '"target": "es2022"',
									},
								],
							},
						],
					},
					result: { output: "success" },
					isError: false,
					status: "completed",
				},
				{
					id: "tc_r3",
					name: "execute",
					args: { command: "npm run typecheck" },
					result: { output: "✓ No type errors found" },
					isError: false,
					status: "completed",
				},
			],
			blocks: [
				{
					type: "thinking",
					text: "I need to understand the current file structure before making changes. Let me read the config file first, then edit it, and finally run the tests.",
				},
				{
					type: "response",
					text: "I'll read, edit, and test the configuration.",
				},
				{ type: "tool", id: "tc_r1" },
				{ type: "tool", id: "tc_r2" },
				{ type: "tool", id: "tc_r3" },
			],
		},
	}),
];

const toolStatesMessages: ParsedMessageEntry[] = [
	createParsedEntry({
		id: 220,
		role: "assistant",
		content: [
			{
				type: "text",
				text: "The command is still running while I inspect the config and retry the deployment edit.",
			},
		],
		parsed: {
			markdown:
				"The command is still running while I inspect the config and retry the deployment edit.",
			toolCalls: [
				{
					id: "tc_state_1",
					name: "execute",
					args: { command: "pnpm run dev" },
				},
				{
					id: "tc_state_2",
					name: "read_file",
					args: { path: "config/app.yml" },
				},
				{
					id: "tc_state_3",
					name: "edit_files",
					args: {
						files: [
							{
								path: "deploy.yml",
								edits: [
									{
										search: "npm run build",
										replace: "pnpm run build",
									},
								],
							},
						],
					},
				},
			],
			toolResults: [
				{
					id: "tc_state_2",
					name: "read_file",
					result: { content: "port: 3000\nmode: development" },
					isError: false,
				},
				{
					id: "tc_state_3",
					name: "edit_files",
					result: { output: "ENOENT: deploy.yml" },
					isError: true,
				},
			],
			tools: [
				{
					id: "tc_state_1",
					name: "execute",
					args: { command: "pnpm run dev" },
					result: {
						output: "Starting dev server...\nWatching for file changes...",
					},
					isError: false,
					status: "running",
				},
				{
					id: "tc_state_2",
					name: "read_file",
					args: { path: "config/app.yml" },
					result: { content: "port: 3000\nmode: development" },
					isError: false,
					status: "completed",
				},
				{
					id: "tc_state_3",
					name: "edit_files",
					args: {
						files: [
							{
								path: "deploy.yml",
								edits: [
									{
										search: "npm run build",
										replace: "pnpm run build",
									},
								],
							},
						],
					},
					result: { output: "ENOENT: deploy.yml" },
					isError: true,
					status: "error",
				},
			],
			blocks: [
				{
					type: "response",
					text: "The command is still running while I inspect the config and retry the deployment edit.",
				},
				{ type: "tool", id: "tc_state_1" },
				{ type: "tool", id: "tc_state_2" },
				{ type: "tool", id: "tc_state_3" },
			],
		},
	}),
];

const longConversationMessages: ParsedMessageEntry[] = [
	createParsedEntry({
		id: 300,
		role: "user",
		createdAt: makeTimestamp(0),
		content: [
			{
				type: "text",
				text: "The save button crashes when the profile form is empty. Can you inspect it?",
			},
		],
		parsed: {
			markdown:
				"The save button crashes when the profile form is empty. Can you inspect it?",
			blocks: [
				{
					type: "response",
					text: "The save button crashes when the profile form is empty. Can you inspect it?",
				},
			],
		},
	}),
	createParsedEntry({
		id: 301,
		role: "assistant",
		createdAt: makeTimestamp(1),
		content: [
			{
				type: "text",
				text: "I found an unchecked user lookup in the save handler.",
			},
		],
		parsed: {
			markdown: "I found an unchecked user lookup in the save handler.",
			toolCalls: [
				{
					id: "tc_conv_1",
					name: "read_file",
					args: { path: "site/src/forms/ProfileForm.tsx" },
				},
			],
			toolResults: [
				{
					id: "tc_conv_1",
					name: "read_file",
					result: {
						content:
							"if (!formState.user.id) {\n  throw new Error('missing user');\n}\n",
					},
					isError: false,
				},
			],
			tools: [
				{
					id: "tc_conv_1",
					name: "read_file",
					args: { path: "site/src/forms/ProfileForm.tsx" },
					result: {
						content:
							"if (!formState.user.id) {\n  throw new Error('missing user');\n}\n",
					},
					isError: false,
					status: "completed",
				},
			],
			blocks: [
				{ type: "tool", id: "tc_conv_1" },
				{
					type: "response",
					text: "I found an unchecked user lookup in the save handler.",
				},
			],
		},
	}),
	createParsedEntry({
		id: 302,
		role: "user",
		createdAt: makeTimestamp(2),
		content: [
			{
				type: "text",
				text: "Please fix it and keep the submit flow the same.",
			},
		],
		parsed: {
			markdown: "Please fix it and keep the submit flow the same.",
			blocks: [
				{
					type: "response",
					text: "Please fix it and keep the submit flow the same.",
				},
			],
		},
	}),
	createParsedEntry({
		id: 303,
		role: "assistant",
		createdAt: makeTimestamp(3),
		content: [
			{
				type: "text",
				text: "I added a guard clause and preserved the existing submit flow.",
			},
		],
		parsed: {
			markdown:
				"I added a guard clause and preserved the existing submit flow.",
			toolCalls: [
				{
					id: "tc_conv_2",
					name: "edit_files",
					args: {
						files: [
							{
								path: "site/src/forms/ProfileForm.tsx",
								edits: [
									{
										search: "throw new Error('missing user');",
										replace:
											"return setError('Please choose a user before saving.');",
									},
								],
							},
						],
					},
				},
			],
			toolResults: [
				{
					id: "tc_conv_2",
					name: "edit_files",
					result: { output: "success" },
					isError: false,
				},
			],
			tools: [
				{
					id: "tc_conv_2",
					name: "edit_files",
					args: {
						files: [
							{
								path: "site/src/forms/ProfileForm.tsx",
								edits: [
									{
										search: "throw new Error('missing user');",
										replace:
											"return setError('Please choose a user before saving.');",
									},
								],
							},
						],
					},
					result: { output: "success" },
					isError: false,
					status: "completed",
				},
			],
			blocks: [
				{ type: "tool", id: "tc_conv_2" },
				{
					type: "response",
					text: "I added a guard clause and preserved the existing submit flow.",
				},
			],
		},
	}),
	createParsedEntry({
		id: 304,
		role: "user",
		createdAt: makeTimestamp(4),
		content: [
			{
				type: "text",
				text: "Can you verify the form still passes its checks?",
			},
		],
		parsed: {
			markdown: "Can you verify the form still passes its checks?",
			blocks: [
				{
					type: "response",
					text: "Can you verify the form still passes its checks?",
				},
			],
		},
	}),
	createParsedEntry({
		id: 305,
		role: "assistant",
		createdAt: makeTimestamp(5),
		content: [
			{
				type: "text",
				text: "I ran the form tests and the submit flow still passes.",
			},
		],
		parsed: {
			markdown: "I ran the form tests and the submit flow still passes.",
			toolCalls: [
				{
					id: "tc_conv_3",
					name: "execute",
					args: { command: "pnpm test ProfileForm" },
				},
			],
			toolResults: [
				{
					id: "tc_conv_3",
					name: "execute",
					result: { output: "✓ ProfileForm validation tests passed" },
					isError: false,
				},
			],
			tools: [
				{
					id: "tc_conv_3",
					name: "execute",
					args: { command: "pnpm test ProfileForm" },
					result: { output: "✓ ProfileForm validation tests passed" },
					isError: false,
					status: "completed",
				},
			],
			blocks: [
				{ type: "tool", id: "tc_conv_3" },
				{
					type: "response",
					text: "I ran the form tests and the submit flow still passes.",
				},
			],
		},
	}),
];

/** Regression guard: a single image attachment must not be duplicated. */
export const UserMessageWithSingleImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Check this screenshot" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "I can see the screenshot. It looks like a settings panel.",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
	},
};

/** Ensures N images in yields exactly N thumbnails with no duplication. */
export const UserMessageWithMultipleImages: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Here are three screenshots" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
					{
						type: "file",
						media_type: "image/jpeg",
						data: TEST_PNG_B64,
					},
					{
						type: "file",
						media_type: "image/webp",
						data: TEST_PNG_B64,
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(3);
	},
};

/** File-id images use a server URL instead of inline base64 data. */
export const UserMessageWithFileIdImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Uploaded via file ID" },
					{
						type: "file",
						media_type: "image/png",
						file_id: "storybook-test-image",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		// Verify file_id path is used, not a base64 data URI.
		expect(images[0]).toHaveAttribute(
			"src",
			"/api/experimental/chats/files/storybook-test-image",
		);
	},
};

/** Text-only messages must not produce spurious image thumbnails. */
export const UserMessageTextOnly: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Just a plain text message" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.queryAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(0);
		expect(canvas.getByText("Just a plain text message")).toBeInTheDocument();
	},
};

/** Assistant-side images go through renderBlockList, not the user path. */
export const AssistantMessageWithImage: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "Generate an image" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{ type: "text", text: "Here is the generated image:" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
	},
};

/** Images and file-references coexist without interfering. */
export const UserMessageWithImagesAndFileRefs: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Look at these files" },
					{
						type: "file",
						media_type: "image/png",
						data: TEST_PNG_B64,
					},
					{
						type: "file-reference",
						file_name: "src/main.go",
						start_line: 10,
						end_line: 25,
						content: 'func main() {\n\tfmt.Println("hello")\n}',
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const images = canvas.getAllByRole("img", { name: "Attached image" });
		expect(images).toHaveLength(1);
		expect(canvas.getByText(/main\.go/)).toBeInTheDocument();
	},
};

/** Usage-limit errors render as an info alert with analytics access. */
export const UsageLimitExceeded: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [],
		detailError: {
			kind: "usage-limit",
			message:
				"You've used $50.00 of your $50.00 spend limit. Your limit resets on July 1, 2025.",
		},
		onOpenAnalytics: fn(),
		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/spend limit/i)).toBeVisible();
		const btn = canvas.getByRole("button", { name: /view usage/i });
		expect(btn).toBeVisible();
		await userEvent.click(btn);
		expect(args.onOpenAnalytics).toHaveBeenCalled();
	},
};

/** Non-usage errors must not show the usage CTA. */
export const GenericErrorDoesNotShowUsageAction: Story = {
	args: {
		...defaultArgs,
		parsedMessages: [],
		detailError: { kind: "generic", message: "Provider request failed." },
		onOpenAnalytics: fn(),
		subagentTitles: new Map(),
		subagentStatusOverrides: new Map(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/provider request failed/i)).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: /view usage/i }),
		).not.toBeInTheDocument();
	},
};

/** File references render inline with text, matching the chat input style. */
export const UserMessageWithInlineFileRef: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Can you refactor " },
					{
						type: "file-reference",
						file_name: "site/src/components/Button.tsx",
						start_line: 42,
						end_line: 42,
						content: "export const Button = ...",
					},
					{ type: "text", text: " to use the new API?" },
				],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "Sure, I'll update that component.",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Button\.tsx/)).toBeInTheDocument();
		expect(canvas.getByText(/Can you refactor/)).toBeInTheDocument();
		expect(canvas.getByText(/to use the new API/)).toBeInTheDocument();
	},
};

/** Multiple file references render inline, no separate section. */
export const UserMessageWithMultipleInlineFileRefs: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [
					{ type: "text", text: "Compare " },
					{
						type: "file-reference",
						file_name: "api/handler.go",
						start_line: 1,
						end_line: 50,
						content: "...",
					},
					{ type: "text", text: " with " },
					{
						type: "file-reference",
						file_name: "api/handler_test.go",
						start_line: 10,
						end_line: 30,
						content: "...",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/handler\.go/)).toBeInTheDocument();
		expect(canvas.getByText(/handler_test\.go/)).toBeInTheDocument();
	},
};

/** Dense markdown exercises headings, tables, and block-level elements. */
export const DenseMarkdown: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 101,
				role: "assistant",
				content: [{ type: "text", text: denseMarkdownText }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", {
				level: 1,
				name: "Transcript rendering overview",
			}),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("heading", {
				level: 2,
				name: "Formatting coverage",
			}),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("heading", {
				level: 3,
				name: "Ordered checklist",
			}),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("heading", {
				level: 4,
				name: "Render block matrix",
			}),
		).toBeInTheDocument();
		expect(canvas.getByRole("table")).toBeInTheDocument();
		expect(canvasElement.querySelector("blockquote")).not.toBeNull();
	},
};

/** Multiple fenced blocks cover long snippets and overflow handling. */
export const LongCodeBlocks: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 102,
				role: "assistant",
				content: [{ type: "text", text: longCodeBlocksText }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/Here are several large snippets/i),
		).toBeInTheDocument();
		expect(canvasElement.querySelectorAll("pre").length).toBeGreaterThanOrEqual(
			4,
		);
	},
};

/** Completed file edits and writes render as expandable tool cards. */
export const DiffHeavyOutput: Story = {
	args: {
		...defaultArgs,
		parsedMessages: diffHeavyOutputMessages,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				/update the Button component and create a new utility file/i,
			),
		).toBeInTheDocument();
		const writeFileButton = await canvas.findByRole("button", {
			name: /Wrote format\.ts/i,
		});
		expect(writeFileButton).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(writeFileButton);
		expect(writeFileButton).toHaveAttribute("aria-expanded", "true");
	},
};

/** Reasoning blocks should render cleanly above a stack of tools. */
export const ReasoningAndToolStack: Story = {
	args: {
		...defaultArgs,
		parsedMessages: reasoningAndToolStackMessages,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/understand the current file structure/i);
		const readFileButton = await canvas.findByRole("button", {
			name: /Read tsconfig\.json/i,
		});
		expect(readFileButton).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(readFileButton);
		expect(readFileButton).toHaveAttribute("aria-expanded", "true");
	},
};

/** Sources and file references should coexist in a single assistant turn. */
export const SourcesAndReferences: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 103,
				role: "user",
				content: [{ type: "text", text: "How does the auth middleware work?" }],
			},
			{
				...baseMessage,
				id: 104,
				role: "assistant",
				content: [
					{
						type: "text",
						text: "# Auth middleware flow\n\nThe middleware extracts the bearer token, validates it, and loads request context before the handler runs. The key helpers are `ExtractAPIKey` and `RequireAPIKey`.",
					},
					{
						type: "source",
						url: "https://github.com/coder/coder/blob/main/coderd/httpmw/auth.go",
						title: "Auth middleware walkthrough",
					},
					{
						type: "source",
						url: "https://github.com/coder/coder/blob/main/coderd/httpapi/auth.go",
						title: "API auth helper reference",
					},
					{
						type: "source",
						url: "https://pkg.go.dev/net/http",
						title: "net/http package docs",
					},
					{
						type: "file-reference",
						file_name: "coderd/httpmw/auth.go",
						start_line: 15,
						end_line: 54,
						content:
							"func ExtractAPIKey(ctx context.Context, r *http.Request) (string, error)",
					},
					{
						type: "file-reference",
						file_name: "coderd/httpapi/auth.go",
						start_line: 88,
						end_line: 132,
						content: "func RequireAPIKey(next http.Handler) http.Handler",
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/ExtractAPIKey/)).toBeInTheDocument();
		const sourcesButton = await canvas.findByRole("button", {
			name: /Searched 3 results/i,
		});
		await userEvent.click(sourcesButton);
		expect(
			await canvas.findByRole("link", { name: /Auth middleware walkthrough/i }),
		).toBeInTheDocument();
		expect(canvas.getByText(/coderd\/httpmw\/auth\.go/)).toBeInTheDocument();
	},
};

/** Running, completed, and error tools can appear in a single stack. */
export const ToolStates: Story = {
	args: {
		...defaultArgs,
		parsedMessages: toolStatesMessages,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("pnpm run dev")).toBeInTheDocument();
		expect(
			await canvas.findByRole("button", { name: /Read app\.yml/i }),
		).toBeInTheDocument();
		expect(
			await canvas.findByRole("button", { name: /Edited deploy\.yml/i }),
		).toBeInTheDocument();
		expect(canvasElement.querySelector(".animate-spin")).not.toBeNull();
		expect(canvasElement.querySelector(".lucide-circle-alert")).not.toBeNull();
	},
};

/** A longer transcript should keep multiple turns readable in order. */
export const LongConversation: Story = {
	args: {
		...defaultArgs,
		parsedMessages: longConversationMessages,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				/The save button crashes when the profile form is empty/i,
			),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/Please fix it and keep the submit flow the same/i),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/Can you verify the form still passes its checks/i),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/I found an unchecked user lookup in the save handler/i),
		).toBeInTheDocument();
		expect(
			canvas.getByText(
				/I added a guard clause and preserved the existing submit flow/i,
			),
		).toBeInTheDocument();
		expect(
			canvas.getByText(
				/I ran the form tests and the submit flow still passes/i,
			),
		).toBeInTheDocument();
	},
};

/** Complex tool cards should still render in a narrow mobile container. */
export const MobileWidth: Story = {
	args: {
		...defaultArgs,
		parsedMessages: diffHeavyOutputMessages,
	},
	decorators: [
		(Story) => (
			<div style={{ width: "100%", maxWidth: "375px" }}>
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(
				/update the Button component and create a new utility file/i,
			),
		).toBeInTheDocument();
		expect(
			await canvas.findByRole("button", { name: /Wrote format\.ts/i }),
		).toBeInTheDocument();
	},
};

/**
 * Verifies the structural requirements for sticky user messages
 * in the flat (section-less) message list:
 * - Each user message renders a data-user-sentinel marker so
 *   the push-up logic can find the next user message via DOM
 *   traversal.
 * - The user message container gets position:sticky.
 * - Sentinels appear in the correct order (matching user
 *   message order).
 */
export const StickyUserMessageStructure: Story = {
	args: {
		...defaultArgs,
		parsedMessages: buildMessages([
			{
				...baseMessage,
				id: 1,
				role: "user",
				content: [{ type: "text", text: "First prompt" }],
			},
			{
				...baseMessage,
				id: 2,
				role: "assistant",
				content: [{ type: "text", text: "First response" }],
			},
			{
				...baseMessage,
				id: 3,
				role: "user",
				content: [{ type: "text", text: "Second prompt" }],
			},
			{
				...baseMessage,
				id: 4,
				role: "assistant",
				content: [{ type: "text", text: "Second response" }],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		// Each user message should produce a data-user-sentinel
		// marker that the push-up scroll logic relies on.
		const sentinels = canvasElement.querySelectorAll("[data-user-sentinel]");
		expect(sentinels.length).toBe(2);

		// Each sentinel should be immediately followed by a sticky
		// container (the user message itself).
		for (const sentinel of sentinels) {
			const container = sentinel.nextElementSibling;
			expect(container).not.toBeNull();
			const style = window.getComputedStyle(container!);
			expect(style.position).toBe("sticky");
		}

		// Sentinels must appear in DOM order matching the message
		// order so nextElementSibling traversal finds the correct
		// next user message.
		const allElements = Array.from(
			canvasElement.querySelectorAll("[data-user-sentinel], [class*='sticky']"),
		);
		const sentinelIndices = Array.from(sentinels).map((s) =>
			allElements.indexOf(s),
		);
		// Sentinels should be in ascending DOM order.
		expect(sentinelIndices[0]).toBeLessThan(sentinelIndices[1]);

		// Both user messages should be visible.
		const canvas = within(canvasElement);
		expect(canvas.getByText("First prompt")).toBeVisible();
		expect(canvas.getByText("Second prompt")).toBeVisible();
	},
};
