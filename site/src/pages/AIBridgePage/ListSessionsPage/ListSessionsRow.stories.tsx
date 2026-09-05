import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import { MockSession } from "#/testHelpers/entities";
import { ListSessionsRow } from "./ListSessionsRow";

const meta: Meta<typeof ListSessionsRow> = {
	title: "pages/AIBridgePage/ListSessionsRow",
	component: ListSessionsRow,
	args: {
		onClick: fn(),
	},
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ListSessionsRow>;

export const Default: Story = {
	args: {
		session: MockSession,
	},
};

export const SingleProvider: Story = {
	args: {
		session: {
			...MockSession,
			providers: ["anthropic"],
		},
	},
};

export const MultipleProviders: Story = {
	args: {
		session: {
			...MockSession,
			providers: ["anthropic", "openai", "copilot"],
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.hover(within(canvasElement).getByText("3 providers"));
		await waitFor(() => {
			const tooltip = screen.getByRole("tooltip");
			expect(tooltip).toHaveTextContent("Providers");
			expect(tooltip).toHaveTextContent("Anthropic");
			expect(tooltip).toHaveTextContent("OpenAI");
			expect(tooltip).toHaveTextContent("GitHub Copilot");
		});
	},
};

export const EmptyProviders: Story = {
	args: {
		session: {
			...MockSession,
			providers: [],
		},
	},
};

export const NullClient: Story = {
	args: {
		session: { ...MockSession, client: null },
	},
};

export const NoInitiatorName: Story = {
	args: {
		session: {
			...MockSession,
			initiator: { ...MockSession.initiator, name: "" },
		},
	},
};

export const LongPrompt: Story = {
	args: {
		session: {
			...MockSession,
			last_prompt:
				"Can you refactor the entire authentication module to use JWT tokens instead of session cookies, and also update all the tests, documentation, and CI pipelines while you're at it?",
		},
	},
};

export const NarrowPromptIcon: Story = {
	args: {
		session: {
			...MockSession,
			last_prompt:
				"Can you refactor the entire authentication module to use JWT tokens instead of session cookies?",
		},
	},
	parameters: {
		viewport: {
			defaultViewport: "mobile2",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("View last prompt")).toBeVisible();
		expect(canvas.getByText(/^Can you refactor/)).not.toBeVisible();
		await userEvent.hover(canvas.getByLabelText("View last prompt"));
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Can you refactor the entire authentication module",
			),
		);
	},
};

export const NoPrompt: Story = {
	args: {
		session: { ...MockSession, last_prompt: undefined },
	},
};

export const ManyThreads: Story = {
	args: {
		session: { ...MockSession, threads: 128 },
	},
};

export const LargeTokenCounts: Story = {
	args: {
		session: {
			...MockSession,
			token_usage_summary: {
				input_tokens: 198_000,
				output_tokens: 32_000,
				cache_read_input_tokens: 150_000,
				cache_write_input_tokens: 12_000,
			},
		},
	},
};

export const NetworkCallsBlocked: Story = {
	args: {
		session: {
			...MockSession,
			network_calls: { total: 23, blocked: 2 },
		},
	},
};

export const NoNetworkActivity: Story = {
	args: {
		session: {
			...MockSession,
			network_calls: { total: 0, blocked: 0 },
		},
	},
};

export const NetworkDisabled: Story = {
	args: {
		session: { ...MockSession, network_calls: undefined },
	},
};
