import type { Meta, StoryObj } from "@storybook/react-vite";
import NotFoundPage from "./NotFoundPage";

const meta: Meta<typeof NotFoundPage> = {
	title: "components/NotFoundPage",
	component: NotFoundPage,
};

export default meta;
type Story = StoryObj<typeof NotFoundPage>;

const Example: Story = {};

export { Example as NotFoundPage };
