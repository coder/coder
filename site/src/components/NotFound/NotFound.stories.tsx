import type { Meta, StoryObj } from "@storybook/react-vite";
import { NotFound } from "./NotFound";

const meta: Meta<typeof NotFound> = {
	title: "components/NotFound",
	component: NotFound,
};

export default meta;
type Story = StoryObj<typeof NotFound>;

const Example: Story = {};

export { Example as NotFound };
