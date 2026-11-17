import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import IconsPage from "./IconsPage";

const meta: Meta<typeof IconsPage> = {
	title: "pages/IconsPage",
	component: IconsPage,
	args: {},
};

export default meta;
type Story = StoryObj<typeof IconsPage>;

const Example: Story = {};

export { Example as IconsPage };

export const NoSearchResults: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("textbox", { name: "Filter" }),
			"no-matching-icon",
		);
	},
};
