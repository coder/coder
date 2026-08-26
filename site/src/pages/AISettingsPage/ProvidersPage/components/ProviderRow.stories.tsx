import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderCopilot,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { ProviderRow } from "./ProviderRow";

const meta: Meta<typeof ProviderRow> = {
	title: "pages/AISettingsPage/ProviderRow",
	component: ProviderRow,
	args: {
		onClick: fn(),
	},
	decorators: [
		(Story) => (
			<Table aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/3">Base URL</TableHead>
						<TableHead className="w-22">Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ProviderRow>;

export const OpenAI: Story = {
	args: {
		provider: MockAIProviderOpenAI,
	},
};

export const Anthropic: Story = {
	args: {
		provider: MockAIProviderAnthropic,
	},
};

export const Bedrock: Story = {
	args: {
		provider: MockAIProviderBedrock,
	},
};

export const LongText: Story = {
	args: {
		provider: {
			...MockAIProviderBedrock,
			name: "bedrock12341234bedrock12341234bedrock12341234",
			display_name: "thisisacoolexample11",
			base_url:
				"https://bedrock-runtime.us-east-2.amazonaws.com/very/long/path/segment",
		},
	},
};

// Copilot is unsupported by Agents, so the row shows the label.
export const NotSupportedInAgents: Story = {
	args: {
		provider: MockAIProviderCopilot,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const badge = canvas.getByRole("button", {
			name: "Not supported in Agents",
		});
		await expect(badge).toBeInTheDocument();
		await userEvent.hover(badge);
		const tooltip = await within(canvasElement.ownerDocument.body).findByRole(
			"tooltip",
		);
		await expect(tooltip).toHaveTextContent(
			"This provider works with the AI Gateway Proxy but Coder Agents can't use it.",
		);
	},
};

export const SupportedHasNoAgentsLabel: Story = {
	args: {
		provider: { ...MockAIProviderOpenAI, enabled: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		await expect(
			canvas.queryByText("Not supported in Agents"),
		).not.toBeInTheDocument();
	},
};

export const WithHostnameCollisionWarning: Story = {
	args: {
		provider: {
			...MockAIProviderOpenAI,
			enabled: true,
			status: {
				warnings: [
					'hostname "api.openai.com" is claimed by provider "first"; not reachable via the AI Gateway Proxy, use direct routing (/api/v2/ai-gateway/openai/...) instead',
				],
			},
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const badge = canvas.getByLabelText(/^Warning: Hostname/);
		await expect(badge).toBeInTheDocument();
		await expect(badge).toHaveAttribute(
			"aria-label",
			expect.stringContaining("api.openai.com"),
		);
		await expect(badge).toHaveAttribute("tabIndex", "0");

		// Hover shows the tooltip with the warning text.
		await userEvent.hover(badge);
		const tooltip = await within(canvasElement.ownerDocument.body).findByRole(
			"tooltip",
		);
		await expect(tooltip).toHaveTextContent("api.openai.com");

		// Keyboard and mouse activation must not navigate the row.
		badge.focus();
		await userEvent.keyboard("{Enter}");
		await userEvent.keyboard(" ");
		await userEvent.click(badge);
		await expect(args.onClick).not.toHaveBeenCalled();
	},
};

export const EmptyWarnings: Story = {
	args: {
		provider: {
			...MockAIProviderOpenAI,
			enabled: true,
			status: { warnings: [] },
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText(/warning/i)).not.toBeInTheDocument();
		// An empty warnings array must not leak a bare "0" into the row.
		await expect(canvas.queryByText("0")).not.toBeInTheDocument();
	},
};
