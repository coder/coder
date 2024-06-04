import DownloadOutlined from "@mui/icons-material/DownloadOutlined";
import Button from "@mui/material/Button";
import { saveAs } from "file-saver";
import type { FC } from "react";
import type { WorkspaceAgent } from "api/typesGenerated";
import type { LineWithID } from "./AgentLogs/AgentLogLine";

type DownloadAgentLogsButtonProps = {
  agent: Pick<WorkspaceAgent, "name" | "status">;
  logs: Pick<LineWithID, "output">[] | undefined;
  onDownload?: (file: Blob, filename: string) => void;
};

export const DownloadAgentLogsButton: FC<DownloadAgentLogsButtonProps> = ({
  agent,
  logs,
  onDownload = saveAs,
}) => {
  const isDisabled =
    agent.status !== "connected" || logs === undefined || logs.length === 0;

  return (
    <Button
      disabled={isDisabled}
      variant="text"
      size="small"
      startIcon={<DownloadOutlined />}
      onClick={() => {
        if (isDisabled) {
          return;
        }
        const text = logs.map((l) => l.output).join("\n");
        const file = new Blob([text], { type: "text/plain" });
        onDownload(file, `${agent.name}-logs.txt`);
      }}
    >
      Download logs
    </Button>
  );
};
