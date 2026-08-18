import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockUserOwner } from "#/testHelpers/entities";
import { UserAutocomplete } from "./UserAutocomplete";

const meta: Meta<typeof UserAutocomplete> = {
	title: "components/UserAutocomplete",
	component: UserAutocomplete,
};

export default meta;
type Story = StoryObj<typeof UserAutocomplete>;

export const WithLabel: Story = {
	args: {
		value: MockUserOwner,
		label: "User",
	},
};

export const NoLabel: Story = {
	args: {
		value: MockUserOwner,
	},
};

// The trigger shows the username, not the email, when the selected user has a
// populated email that differs from the username. Pins the username-primary
// trigger: a revert to an email-primary label fails this assertion.
export const SelectedShowsUsernameNotEmail: Story = {
	args: {
		value: { ...MockUserOwner, email: "someone-else@example.com" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");
		expect(trigger).toHaveTextContent(MockUserOwner.username);
		expect(trigger).not.toHaveTextContent("someone-else@example.com");
	},
};

// The trigger label falls back to the username, never blank, even when the
// selected user has an empty email.
export const SelectedWithoutEmail: Story = {
	args: {
		value: { ...MockUserOwner, email: "" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");
		expect(trigger).toHaveTextContent(MockUserOwner.username);
		expect(trigger).not.toHaveTextContent("Select a user");
	},
};
