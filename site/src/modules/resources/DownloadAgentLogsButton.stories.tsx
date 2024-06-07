import type { Meta, StoryObj } from "@storybook/react";
import { waitFor, within, userEvent, expect, fn } from "@storybook/test";
import { agentLogsKey } from "api/queries/workspaces";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { DownloadAgentLogsButton } from "./DownloadAgentLogsButton";

const meta: Meta<typeof DownloadAgentLogsButton> = {
  title: "modules/resources/DownloadAgentLogsButton",
  component: DownloadAgentLogsButton,
  args: {
    workspaceId: MockWorkspace.id,
    agent: MockWorkspaceAgent,
  },
  parameters: {
    queries: [
      {
        key: agentLogsKey(MockWorkspace.id, MockWorkspaceAgent.id),
        data: generateLogs(5),
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof DownloadAgentLogsButton>;

export const Default: Story = {};

export const ClickOnDownload: Story = {
  args: {
    download: fn(),
  },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Download logs" }),
    );
    await waitFor(() =>
      expect(args.download).toHaveBeenCalledWith(
        expect.anything(),
        `${MockWorkspaceAgent.name}-logs.txt`,
      ),
    );
    const blob: Blob = (args.download as jest.Mock).mock.calls[0][0];
    await expect(blob.type).toEqual("text/plain");
  },
};

function generateLogs(count: number): WorkspaceAgentLog[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i,
    output: `log line ${i}`,
    created_at: new Date().toISOString(),
    level: "info",
    source_id: "",
  }));
}
