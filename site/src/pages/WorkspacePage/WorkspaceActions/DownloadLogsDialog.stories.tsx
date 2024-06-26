import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, fn, waitFor, within } from "@storybook/test";
import { agentLogsKey, buildLogsKey } from "api/queries/workspaces";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { withDesktopViewport } from "testHelpers/storybook";
import { DownloadLogsDialog } from "./DownloadLogsDialog";

const meta: Meta<typeof DownloadLogsDialog> = {
  title: "pages/WorkspacePage/DownloadLogsDialog",
  component: DownloadLogsDialog,
  args: {
    open: true,
    workspace: MockWorkspace,
    onClose: fn(),
  },
  parameters: {
    queries: [
      {
        key: buildLogsKey(MockWorkspace.id),
        data: generateLogs(200),
      },
      {
        key: agentLogsKey(MockWorkspace.id, MockWorkspaceAgent.id),
        data: generateLogs(400),
      },
    ],
  },
  decorators: [withDesktopViewport],
};

export default meta;
type Story = StoryObj<typeof DownloadLogsDialog>;

export const Ready: Story = {};

export const Loading: Story = {
  parameters: {
    queries: [
      {
        key: buildLogsKey(MockWorkspace.id),
        data: undefined,
      },
      {
        key: agentLogsKey(MockWorkspace.id, MockWorkspaceAgent.id),
        data: undefined,
      },
    ],
  },
};

export const DownloadLogs: Story = {
  args: {
    download: fn(),
  },
  play: async ({ args }) => {
    const screen = within(document.body);
    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    await waitFor(() =>
      expect(args.download).toHaveBeenCalledWith(
        expect.anything(),
        `${MockWorkspace.name}-logs.zip`,
      ),
    );
    const blob: Blob = (args.download as jest.Mock).mock.calls[0][0];
    await expect(blob.type).toEqual("application/zip");
  },
};

function generateLogs(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    output: `log ${i + 1}`,
  }));
}
