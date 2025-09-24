import {
	MockNonClassicParameterFlowWorkspace,
	MockTemplateVersionParameter6,
	MockWorkspace,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { RetryButton } from "./RetryButton";

const meta: Meta<typeof RetryButton> = {
	title: "pages/WorkspacePage/RetryButton",
	component: RetryButton,
};

export default meta;
type Story = StoryObj<typeof RetryButton>;

export const Default: Story = {};

export const WithBuildParameters: Story = {
	args: {
		enableBuildParameters: true,
		workspace: MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: ["workspace", MockWorkspace.id, "parameters"],
				data: { templateVersionRichParameters: [], buildParameters: [] },
			},
		],
	},
};

export const WithOpenBuildParameters: Story = {
	args: {
		enableBuildParameters: true,
		workspace: MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: ["workspace", MockWorkspace.id, "parameters"],
				data: { templateVersionRichParameters: [], buildParameters: [] },
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			await waitFor(() =>
				expect(screen.getByText("Build Options")).toBeInTheDocument(),
			);
		});
	},
};

export const WithOpenEphemeralBuildParameters: Story = {
	args: {
		enableBuildParameters: true,
		workspace: MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: ["workspace", MockWorkspace.id, "parameters"],
				data: {
					templateVersionRichParameters: [MockTemplateVersionParameter6],
					buildParameters: [],
				},
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			await waitFor(() =>
				expect(
					screen.getByText(
						"These parameters only apply for a single workspace start.",
					),
				).toBeInTheDocument(),
			);
		});
	},
};

export const WithOpenEphemeralBuildParametersNotClassic: Story = {
	args: {
		enableBuildParameters: true,
		workspace: MockNonClassicParameterFlowWorkspace,
	},
	parameters: {
		queries: [
			{
				key: [
					"workspace",
					MockNonClassicParameterFlowWorkspace.id,
					"parameters",
				],
				data: {
					templateVersionRichParameters: [MockTemplateVersionParameter6],
					buildParameters: [],
				},
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			await waitFor(() =>
				expect(
					screen.getByText(
						"This workspace has ephemeral parameters which may use a temporary value on workspace start. Configure the following parameters in workspace settings.",
					),
				).toBeInTheDocument(),
			);
		});
	},
};
