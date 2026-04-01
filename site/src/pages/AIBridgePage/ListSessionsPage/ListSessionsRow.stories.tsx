import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
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
			},
		},
	},
};
