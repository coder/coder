import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const meta: Meta<typeof CreateOrganizationPageView> = {
	title: "pages/CreateOrganizationPageView",
	component: CreateOrganizationPageView,
	args: {
		isEntitled: true,
	},
};

export default meta;
type Story = StoryObj<typeof CreateOrganizationPageView>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The badge is passive: hovering it must not surface a paywall.
		await userEvent.hover(canvas.getByText("Premium"));
		await expect(
			screen.queryByRole("link", { name: "Read the documentation" }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("link", { name: "Read the documentation" }),
		).toBeVisible();
	},
};

export const WithError: Story = {
	args: { error: "Oh no!" },
};

export const InvalidName: Story = {
	args: {
		error: mockApiError({
			message: "Display name is bad",
			validations: [
				{
					field: "display_name",
					detail: "That display name is terrible. What were you thinking?",
				},
			],
		}),
	},
};
