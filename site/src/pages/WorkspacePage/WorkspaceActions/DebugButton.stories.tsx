import {
	MockTemplateVersionParameter1,
	MockWorkspace,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { richParametersKey } from "api/queries/templates";
import { workspaceBuildParametersKey } from "api/queries/workspaceBuilds";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { DebugButton } from "./DebugButton";

const meta: Meta<typeof DebugButton> = {
	title: "pages/WorkspacePage/DebugButton",
	component: DebugButton,
};

export default meta;
type Story = StoryObj<typeof DebugButton>;

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
			await userEvent.click(screen.getByText("Debug with build parameters"));
			await waitFor(() =>
				expect(screen.getByText("Build Options")).toBeInTheDocument(),
			);
		});
	},
};
