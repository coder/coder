import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { mockApiError } from "testHelpers/entities";
import { CreateGroupPageView } from "./CreateGroupPageView";

const meta: Meta<typeof CreateGroupPageView> = {
	title: "pages/OrganizationGroupsPage/CreateGroupPageView",
	component: CreateGroupPageView,
};

export default meta;
type Story = StoryObj<typeof CreateGroupPageView>;

export const Example: Story = {};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "A group named new-group already exists.",
			validations: [{ field: "name", detail: "Group names must be unique" }],
		}),
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Enter name", async () => {
			const input = canvas.getByLabelText("Name");
			await userEvent.type(input, "new-group");
			input.blur();
		});
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name");
		await user.type(input, "$om3 !nv@lid Name");
		input.blur();
	},
};
