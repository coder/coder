import {
	MockNonClassicParameterFlowWorkspace,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter6,
	MockWorkspace,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { richParametersKey } from "api/queries/templates";
import { workspaceBuildParametersKey } from "api/queries/workspaceBuilds";
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
				key: richParametersKey(MockWorkspace.latest_build.template_version_id),
				data: [],
			},
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [],
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
				key: richParametersKey(MockWorkspace.latest_build.template_version_id),
				data: [MockTemplateVersionParameter1],
			},
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [],
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			await waitFor(() =>
				expect(screen.getByText("Build options")).toBeInTheDocument(),
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
				key: richParametersKey(MockWorkspace.latest_build.template_version_id),
				data: [MockTemplateVersionParameter6],
			},
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [],
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			expect(
				await screen.findByText(
					"These parameters only apply for a single workspace start.",
				),
			).toBeInTheDocument();
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
				key: richParametersKey(MockWorkspace.latest_build.template_version_id),
				data: [MockTemplateVersionParameter6],
			},
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [],
			},
		],
	},
	play: async ({ step }) => {
		await step("open popover", async () => {
			await userEvent.click(screen.getByText("Retry with build parameters"));
			expect(
				await screen.findByText(
					"This workspace has ephemeral parameters which may use a temporary value on workspace start. Configure the following parameters in workspace settings.",
				),
			).toBeInTheDocument();
		});
	},
};
