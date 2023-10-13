import { CliAuthPageView } from "./CliAuthPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof CliAuthPageView> = {
  title: "pages/CliAuthPage",
  component: CliAuthPageView,
  args: {
    sessionToken: "some-session-token",
  },
};

export default meta;
type Story = StoryObj<typeof CliAuthPageView>;

const Example: Story = {};

export { Example as CliAuthPage };
