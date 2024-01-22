import { MockBuildInfo, MockUser } from "testHelpers/entities";
import { UserDropdown } from "./UserDropdown";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UserDropdown> = {
  title: "components/UserDropdown",
  component: UserDropdown,
  args: {
    user: MockUser,
    isDefaultOpen: true,
    buildInfo: MockBuildInfo,
    supportLinks: [
      { icon: "docs", name: "Documentation", target: "" },
      { icon: "bug", name: "Report a bug", target: "" },
      { icon: "chat", name: "Join the Coder Discord", target: "" },
      { icon: "/icon/aws.svg", name: "Amazon Web Services", target: "" },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof UserDropdown>;

const Example: Story = {};

export { Example as UserDropdown };
