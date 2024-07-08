import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, waitFor, within, expect } from "@storybook/test";
import { MockWorkspace } from "testHelpers/entities";
import { DebugButton } from "./DebugButton";

const meta: Meta<typeof DebugButton> = {
  title: "pages/WorkspacePage/DebugButton",
  component: DebugButton,
};

export default meta;
type Story = StoryObj<typeof DebugButton>;

export const Default: Story = {};

export const WithBuildParameters: Story = {
  args: {
    enableBuildParameters: true,
    workspace: MockWorkspace,
  },
  parameters: {
    queries: [
      {
        key: ["workspace", MockWorkspace.id, "parameters"],
        data: { templateVersionRichParameters: [], buildParameters: [] },
      },
    ],
  },
};

export const WithOpenBuildParameters: Story = {
  args: {
    enableBuildParameters: true,
    workspace: MockWorkspace,
  },
  parameters: {
    queries: [
      {
        key: ["workspace", MockWorkspace.id, "parameters"],
        data: { templateVersionRichParameters: [], buildParameters: [] },
      },
    ],
  },
  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("open popover", async () => {
      await userEvent.click(screen.getByTestId("build-parameters-button"));
      await waitFor(() =>
        expect(screen.getByText("Build Options")).toBeInTheDocument(),
      );
    });
  },
};
