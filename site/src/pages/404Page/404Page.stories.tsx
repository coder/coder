import type { Meta, StoryObj } from "@storybook/react";
import NotFoundPage from "./404Page";

const meta: Meta<typeof NotFoundPage> = {
  title: "components/NotFoundPage",
  component: NotFoundPage,
};

export default meta;
type Story = StoryObj<typeof NotFoundPage>;

const Example: Story = {};
export { Example as NotFoundPage };
