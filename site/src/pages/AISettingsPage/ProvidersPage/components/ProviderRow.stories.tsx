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
			<Table className="table-fixed" aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-[42%]">Name</TableHead>
						<TableHead className="w-[38%]">Base URL</TableHead>
						<TableHead className="w-20 text-center">
							<span className="sr-only">Status</span>
						</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open provider</span>
						</TableHead>
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
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const badge = canvas.getByText("Not supported in Agents");
		await expect(badge).toBeInTheDocument();

		// Hover shows the explanation in the shared tooltip, matching the
		// warning badge instead of the native title tooltip.
		await userEvent.hover(badge);
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(/AI Gateway proxy/);

		// Activation must not navigate the row.
		badge.focus();
		await userEvent.keyboard("{Enter}");
		await userEvent.keyboard(" ");
		await userEvent.click(badge);
		await expect(args.onClick).not.toHaveBeenCalled();
	},
};

export const Disabled: Story = {
	args: {
		provider: { ...MockAIProviderOpenAI, enabled: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
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
		await expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();
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
		const badge = canvas.getByText(/warning/i);
		await expect(badge).toBeInTheDocument();
		await expect(badge).toHaveAttribute(
			"aria-label",
			expect.stringContaining("api.openai.com"),
		);
		await expect(badge).toHaveAttribute("tabIndex", "0");

		// Hover shows the tooltip with the warning text.
		await userEvent.hover(badge);
		await expect(
			await canvas.findByText(/api\.openai\.com/, {}, { timeout: 2000 }),
		).toBeInTheDocument();

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
