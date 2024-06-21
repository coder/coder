import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { useMemo, useState, type FC } from "react";
import { useQueries, useQuery } from "react-query";
import { agentLogs, buildLogs } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import {
  ConfirmDialog,
  type ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";

type DownloadLogsDialogProps = Pick<
  ConfirmDialogProps,
  "onConfirm" | "onClose" | "open"
> & {
  workspace: Workspace;
  download?: (zip: Blob, filename: string) => void;
};

type DownloadableFile = {
  name: string;
  blob: Blob | undefined;
};

export const DownloadLogsDialog: FC<DownloadLogsDialogProps> = ({
  workspace,
  download = saveAs,
  ...dialogProps
}) => {
  const theme = useTheme();
  const agents = selectAgents(workspace);
  const agentLogResults = useQueries({
    queries: agents.map((a) => ({
      ...agentLogs(workspace.id, a.id),
      enabled: dialogProps.open,
    })),
  });
  const buildLogsQuery = useQuery({
    ...buildLogs(workspace),
    enabled: dialogProps.open,
  });
  const downloadableFiles: DownloadableFile[] = useMemo(() => {
    const files: DownloadableFile[] = [
      {
        name: `${workspace.name}-build-logs.txt`,
        blob: buildLogsQuery.data
          ? new Blob([buildLogsQuery.data.map((l) => l.output).join("\n")], {
              type: "text/plain",
            })
          : undefined,
      },
    ];

    agents.forEach((a, i) => {
      const name = `${a.name}-logs.txt`;
      const logs = agentLogResults[i].data;
      const txt = logs?.map((l) => l.output).join("\n");
      let blob: Blob | undefined;
      if (txt) {
        blob = new Blob([txt], { type: "text/plain" });
      }
      files.push({ name, blob });
    });

    return files;
  }, [agentLogResults, agents, buildLogsQuery.data, workspace.name]);
  const isLoadingFiles = downloadableFiles.some((f) => f.blob === undefined);
  const [isDownloading, setIsDownloading] = useState(false);

  return (
    <ConfirmDialog
      {...dialogProps}
      hideCancel={false}
      title="Download logs"
      confirmText="Download"
      disabled={isLoadingFiles}
      confirmLoading={isDownloading}
      onConfirm={async () => {
        try {
          setIsDownloading(true);
          const zip = new JSZip();
          downloadableFiles.forEach((f) => {
            if (f.blob) {
              zip.file(f.name, f.blob);
            }
          });
          const content = await zip.generateAsync({ type: "blob" });
          download(content, `${workspace.name}-logs.zip`);
          dialogProps.onClose();
          setTimeout(() => {
            setIsDownloading(false);
          }, theme.transitions.duration.leavingScreen);
        } catch (error) {
          setIsDownloading(false);
          displayError("Error downloading workspace logs");
          console.error(error);
        }
      }}
      description={
        <Stack css={{ paddingBottom: 16 }}>
          <p>
            Downloading logs will create a zip file containing all logs from all
            jobs in this workspace. This may take a while.
          </p>
          <ul css={styles.list}>
            {downloadableFiles.map((f) => (
              <li key={f.name} css={styles.listItem}>
                <span css={styles.listItemPrimary}>{f.name}</span>
                <span css={styles.listItemSecondary}>
                  {f.blob ? (
                    humanBlobSize(f.blob.size)
                  ) : (
                    <Skeleton variant="text" width={48} height={12} />
                  )}
                </span>
              </li>
            ))}
          </ul>
        </Stack>
      }
    />
  );
};

function humanBlobSize(size: number) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (size > 1024 && i < units.length) {
    size /= 1024;
    i++;
  }
  return `${size.toFixed(2)} ${units[i]}`;
}

function selectAgents(workspace: Workspace): WorkspaceAgent[] {
  return workspace.latest_build.resources
    .flatMap((r) => r.agents)
    .filter((a) => a !== undefined) as WorkspaceAgent[];
}

const styles = {
  list: {
    listStyle: "none",
    padding: 0,
    margin: 0,
    display: "flex",
    flexDirection: "column",
    gap: 8,
  },
  listItem: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
  },
  listItemPrimary: (theme) => ({
    fontWeight: 500,
    color: theme.palette.text.primary,
  }),
  listItemSecondary: {
    fontSize: 14,
  },
} satisfies Record<string, Interpolation<Theme>>;
