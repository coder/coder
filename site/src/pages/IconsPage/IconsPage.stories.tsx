import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { IconsPage } from "./IconsPage";

const meta: Meta<typeof IconsPage> = {
  title: "pages/IconsPage",
  parameters: { chromatic },
  component: IconsPage,
  args: {},
};

export default meta;
type Story = StoryObj<typeof IconsPage>;

const Example: Story = {};

export { Example as IconsPage };
