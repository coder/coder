import {
	MockWorkspaceAgent,
	MockWorkspaceAgentDevcontainer,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { AgentDevcontainerMoreActions } from "./AgentDevcontainerMoreActions";

const meta: Meta<typeof AgentDevcontainerMoreActions> = {
	title: "modules/resources/AgentDevcontainerMoreActions",
	component: AgentDevcontainerMoreActions,
	args: {
		parentAgent: MockWorkspaceAgent,
		devcontainer: MockWorkspaceAgentDevcontainer,
	},
};

export default meta;
type Story = StoryObj<typeof AgentDevcontainerMoreActions>;

export const Default: Story = {};

export const MenuOpen: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		await user.click(
			canvas.getByRole("button", { name: "Dev Container actions" }),
		);

		const body = canvasElement.ownerDocument.body;
		await within(body).findByText("Delete…");
	},
};

export const ConfirmDialogOpen: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		await user.click(
			canvas.getByRole("button", { name: "Dev Container actions" }),
		);

		const body = canvasElement.ownerDocument.body;
		await user.click(await within(body).findByText("Delete…"));

		await within(body).findByText("Delete Dev Container");
	},
};

export const ConfirmDeleteCallsAPI: Story = {
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		const deleteSpy = spyOn(API, "deleteDevContainer").mockResolvedValue(
			undefined as never,
		);

		await user.click(
			canvas.getByRole("button", { name: "Dev Container actions" }),
		);

		const body = canvasElement.ownerDocument.body;
		await user.click(await within(body).findByText("Delete…"));

		await user.click(within(body).getByTestId("confirm-button"));

		await waitFor(() => {
			expect(deleteSpy).toHaveBeenCalledTimes(1);
			expect(deleteSpy).toHaveBeenCalledWith({
				parentAgentId: args.parentAgent.id,
				devcontainerId: args.devcontainer.id,
			});
		});
	},
};
