import type { Meta, StoryObj } from "@storybook/react";
import { expect, screen, userEvent, within, waitFor } from "@storybook/test";
import { MockBuildInfo, MockUser } from "testHelpers/entities";
import { UserDropdown } from "./UserDropdown";

const meta: Meta<typeof UserDropdown> = {
  title: "modules/dashboard/UserDropdown",
  component: UserDropdown,
  args: {
    user: MockUser,
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

const Example: Story = {
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("click to open", async () => {
      await userEvent.click(canvas.getByRole("button"));
      await waitFor(() =>
        expect(screen.getByText(/v99\.999\.9999/i)).toBeInTheDocument(),
      );
    });
  },
};

export { Example as UserDropdown };
