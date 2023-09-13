import { CliAuthPageView } from "./CliAuthPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof CliAuthPageView> = {
  title: "pages/CliAuthPageView",
  component: CliAuthPageView,
  args: {
    sessionToken: "some-session-token",
  },
};

export default meta;
type Story = StoryObj<typeof CliAuthPageView>;

export const Example: Story = {};
