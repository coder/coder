import type { Meta, StoryObj } from "@storybook/react";
import { waitFor, within, userEvent, expect, fn } from "@storybook/test";
import { MockWorkspaceAgent } from "testHelpers/entities";
import { DownloadAgentLogsButton } from "./DownloadAgentLogsButton";

const meta: Meta<typeof DownloadAgentLogsButton> = {
  title: "modules/resources/DownloadAgentLogsButton",
  component: DownloadAgentLogsButton,
  args: {
    agent: MockWorkspaceAgent,
    logs: generateLogs(10),
  },
};

export default meta;
type Story = StoryObj<typeof DownloadAgentLogsButton>;

export const Default: Story = {};

export const ClickOnDownload: Story = {
  args: {
    onDownload: fn(),
  },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Download" }));
    await waitFor(() =>
      expect(args.onDownload).toHaveBeenCalledWith(
        expect.anything(),
        `${MockWorkspaceAgent.name}-logs.txt`,
      ),
    );
    const blob: Blob = (args.onDownload as jest.Mock).mock.calls[0][0];
    await expect(blob.type).toEqual("text/plain");
  },
};

function generateLogs(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    output: `log line ${i}`,
  }));
}
