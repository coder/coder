import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { MockUserMember } from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { UserActionDialogs } from "./UserActionDialogs";

const meta: Meta<typeof UserActionDialogs> = {
	title: "modules/users/UserActionDialogs",
	component: UserActionDialogs,
	decorators: [withToaster],
	args: {
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UserActionDialogs>;

export const Suspend: Story = {
	args: {
		action: { type: "suspend", user: MockUserMember },
	},
	beforeEach: () => {
		spyOn(API, "suspendUser").mockResolvedValue({
			...MockUserMember,
			status: "suspended",
		});
	},
	play: async () => {
		const dialog = await within(document.body).findByRole("dialog");
		await within(dialog).findByRole("heading", { name: "Suspend user" });
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Suspend" }),
		);
		await within(document.body).findByText(/suspended successfully/);
	},
};

export const Closed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByRole("dialog")).not.toBeInTheDocument();
		await expect(
			within(document.body).queryByRole("dialog"),
		).not.toBeInTheDocument();
	},
};
