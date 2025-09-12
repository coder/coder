import { MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
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
