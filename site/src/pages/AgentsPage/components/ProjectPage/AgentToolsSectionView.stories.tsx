import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { AgentToolsSectionView } from "./AgentToolsSection";

const meta: Meta<typeof AgentToolsSectionView> = {
	title: "pages/AgentsPage/ProjectPage/AgentToolsSectionView",
	component: AgentToolsSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		attached: [
			{
				id: "github",
				label: "GitHub (security bot)",
				iconURL: "/icon/github.svg",
			},
			{ id: "linear", label: "Linear", iconURL: "" },
		],
		available: [{ id: "slack", label: "Slack", iconURL: "" }],
		isReady: true,
		error: undefined,
		onRetry: fn(),
		canEdit: true,
		isSaving: false,
		saveError: undefined,
		onAdd: fn(),
		onRemove: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentToolsSectionView>;

export const Default: Story = {};

export const Empty: Story = { args: { attached: [] } };

export const Saving: Story = { args: { isSaving: true } };

export const ReadOnly: Story = { args: { canEdit: false } };

export const AddMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Add MCP server" }),
		);
	},
};

export const NothingLeftToAdd: Story = {
	args: { available: [] },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Add MCP server" }),
		);
	},
};

export const NoServersConfigured: Story = {
	args: { attached: [], available: [] },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Add MCP server" }),
		);
	},
};

export const Loading: Story = { args: { isReady: false } };

export const LoadError: Story = {
	args: {
		isReady: false,
		error: mockApiError({ message: "Failed to load MCP servers." }),
	},
};

export const RefetchError: Story = {
	args: {
		error: mockApiError({ message: "Failed to refresh MCP servers." }),
	},
};

export const SaveError: Story = {
	args: {
		saveError: mockApiError({ message: "Failed to save changes." }),
	},
};

export const AddMenuNoMatch: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Add MCP server" }),
		);
		await userEvent.keyboard("zzz");
	},
};

/** A server that needs the agent's token links to Connected accounts. */
export const TokenNeeded: Story = {
	args: {
		attached: [
			{
				id: "slack",
				label: "Slack",
				iconURL: "/icon/slack.svg",
				needsToken: true,
			},
		],
	},
};
