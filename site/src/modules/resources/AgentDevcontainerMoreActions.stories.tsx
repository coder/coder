import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { AgentDevcontainerMoreActions } from "./AgentDevcontainerMoreActions";

const meta: Meta<typeof AgentDevcontainerMoreActions> = {
	title: "modules/resources/AgentDevcontainerMoreActions",
	component: AgentDevcontainerMoreActions,
	args: {},
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
	},
};

export const ConfirmDeleteCallsAPI: Story = {
	args: {
		deleteDevContainer: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		await user.click(
			canvas.getByRole("button", { name: "Dev Container actions" }),
		);

		const body = canvasElement.ownerDocument.body;
		await user.click(await within(body).findByText("Delete…"));

		await user.click(within(body).getByTestId("confirm-button"));

		await waitFor(() => {
			expect(args.deleteDevContainer).toHaveBeenCalledTimes(1);
		});
	},
};
