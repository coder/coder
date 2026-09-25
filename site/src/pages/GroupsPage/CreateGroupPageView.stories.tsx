import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { CreateGroupPageView } from "./CreateGroupPageView";

const meta: Meta<typeof CreateGroupPageView> = {
	title: "pages/OrganizationGroupsPage/CreateGroupPageView",
	component: CreateGroupPageView,
	args: {
		onSubmit: fn(),
		onCancel: fn(),
		isLoading: false,
		showOrganizations: true,
	},
};

export default meta;
type Story = StoryObj<typeof CreateGroupPageView>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "New group" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Back to groups" }),
		).toBeVisible();
		await expect(canvas.getByRole("textbox", { name: /^Name/ })).toBeRequired();
		await expect(canvas.getByText("Unique identifier.")).toBeVisible();
		await expect(
			canvas.getByText("Friendly name. Defaults to the name if blank."),
		).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Save" })).toBeVisible();
		await expect(
			canvas.getByText(/Add a group to this organization/),
		).toBeVisible();
	},
};

export const WithoutOrganizations: Story = {
	args: {
		showOrganizations: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Add a group to this deployment/),
		).toBeVisible();
	},
};

export const Cancel: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Cancel" }));
		await expect(args.onCancel).toHaveBeenCalled();
	},
};

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
			const input = canvas.getByRole("textbox", { name: /^Name/ });
			await userEvent.type(input, "new-group");
			input.blur();
		});
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByRole("textbox", { name: /^Name/ });
		await user.type(input, "$om3 !nv@lid Name");
		input.blur();
	},
};
