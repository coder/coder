import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { useMemo, useState, type FC, useRef, useEffect } from "react";
import { UseQueryOptions, useQueries, useQuery } from "react-query";
import { agentLogs, buildLogs } from "api/queries/workspaces";
import type {
  Workspace,
  WorkspaceAgent,
  WorkspaceAgentLog,
} from "api/typesGenerated";
import {
  ConfirmDialog,
  type ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";

const BLOB_SIZE_UNITS = ["B", "KB", "MB", "GB", "TB"] as const;

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
  open,
  onConfirm,
  onClose,
}) => {
  const theme = useTheme();

  const buildLogsQuery = useQuery({
    ...buildLogs(workspace),
    enabled: open,
  });
  const buildLogsFile = useMemo<DownloadableFile>(() => {
    return {
      name: `${workspace.name}-build-logs.txt`,
      blob: buildLogsQuery.data
        ? new Blob([buildLogsQuery.data.map((l) => l.output).join("\n")], {
            type: "text/plain",
          })
        : undefined,
    };
  }, [workspace.name, buildLogsQuery.data]);

  // This is clunky, but we have to memoize in two steps to make sure that we
  // don't accidentally break the memo cache every render. We can't tuck
  // everything into a single memo call, because we need to set up React Query
  // state between processing the agents, and we can't violate rules of hooks
  type AgentInfo = Readonly<{
    agents: readonly WorkspaceAgent[];
    queries: readonly UseQueryOptions<readonly WorkspaceAgentLog[]>[];
  }>;

  const { agents, queries } = useMemo<AgentInfo>(() => {
    const allAgents = workspace.latest_build.resources.flatMap(
      (resource) => resource.agents ?? [],
    );

    // Can't use the "new Set()" trick because we're not dealing with primitives
    const uniqueAgents = [
      ...new Map(allAgents.map((agent) => [agent.id, agent])).values(),
    ];

    return {
      agents: uniqueAgents,
      queries: uniqueAgents.map((agent) => {
        return {
          ...agentLogs(workspace.id, agent.id),
          enabled: open,
        };
      }),
    };
  }, [workspace, open]);

  const agentLogResults = useQueries({ queries });
  const allFiles = useMemo<readonly DownloadableFile[]>(() => {
    const files: DownloadableFile[] = [buildLogsFile];

    agents.forEach((a, i) => {
      const name = `${a.name}-logs.txt`;
      const txt = agentLogResults[i]?.data?.map((l) => l.output).join("\n");

      let blob: Blob | undefined;
      if (txt) {
        blob = new Blob([txt], { type: "text/plain" });
      }

      files.push({ name, blob });
    });

    return files;
  }, [agentLogResults, agents, buildLogsFile]);

  const [isDownloading, setIsDownloading] = useState(false);
  const timeoutIdRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    const clearTimeoutOnUnmount = () => {
      window.clearTimeout(timeoutIdRef.current);
    };

    return clearTimeoutOnUnmount;
  }, []);

  const isWorkspaceHealthy = workspace.health.healthy;
  const isLoadingFiles = allFiles.some((f) => f.blob === undefined);

  return (
    <ConfirmDialog
      open={open}
      onClose={onClose}
      hideCancel={false}
      title="Download logs"
      confirmLoading={isDownloading}
      confirmText={
        <>
          Download
          {!isWorkspaceHealthy && <> {isLoadingFiles ? "partial" : "all"}</>}
        </>
      }
      disabled={
        isDownloading ||
        // If a workspace isn't healthy, let the user download as many logs as
        // they can
        (isWorkspaceHealthy && isLoadingFiles)
      }
      onConfirm={async () => {
        setIsDownloading(true);
        const zip = new JSZip();
        allFiles.forEach((f) => {
          if (f.blob) {
            zip.file(f.name, f.blob);
          }
        });

        try {
          const content = await zip.generateAsync({ type: "blob" });
          download(content, `${workspace.name}-logs.zip`);
          onClose();

          timeoutIdRef.current = window.setTimeout(() => {
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

          {!isWorkspaceHealthy && isLoadingFiles && (
            <>
              <ErrorAlert
                error="Your workspace is not healthy. Some logs may not be available, but
              you can still download any that are."
              />
            </>
          )}

          <ul css={styles.list}>
            {allFiles.map((f) => (
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
  let i = 0;
  while (size > 1024 && i < BLOB_SIZE_UNITS.length) {
    size /= 1024;
    i++;
  }

  const finalUnit = BLOB_SIZE_UNITS[i] ?? BLOB_SIZE_UNITS.at(-1) ?? "TB";
  return `${size.toFixed(2)} ${finalUnit}`;
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
