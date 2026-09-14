import type { Meta, StoryObj } from "@storybook/react-vite";
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
