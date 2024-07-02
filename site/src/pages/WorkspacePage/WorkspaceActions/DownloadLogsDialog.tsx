import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { type FC, useMemo, useState, useRef, useEffect } from "react";
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
  open,
  onClose,
  download = saveAs,
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
  // state between processing the agents, but we can't violate rules of hooks by
  // putting hooks inside of hooks
  type AgentInfo = Readonly<{
    agents: readonly WorkspaceAgent[];
    logOptionsArray: readonly UseQueryOptions<readonly WorkspaceAgentLog[]>[];
  }>;

  const { agents, logOptionsArray } = useMemo<AgentInfo>(() => {
    const allAgents = workspace.latest_build.resources.flatMap(
      (resource) => resource.agents ?? [],
    );

    // Can't use the "new Set()" trick because we're not dealing with primitives
    const uniqueAgents = [
      ...new Map(allAgents.map((agent) => [agent.id, agent])).values(),
    ];

    return {
      agents: uniqueAgents,
      logOptionsArray: uniqueAgents.map((agent) => {
        return {
          ...agentLogs(workspace.id, agent.id),
          enabled: open,
        };
      }),
    };
  }, [workspace, open]);

  const agentLogQueries = useQueries({ queries: logOptionsArray });
  const allFiles = useMemo<readonly DownloadableFile[]>(() => {
    const files: DownloadableFile[] = [buildLogsFile];

    agents.forEach((a, i) => {
      const name = `${a.name}-logs.txt`;
      const txt = agentLogQueries[i]?.data?.map((l) => l.output).join("\n");

      let blob: Blob | undefined;
      if (txt) {
        blob = new Blob([txt], { type: "text/plain" });
      }

      files.push({ name, blob });
    });

    return files;
  }, [agentLogQueries, agents, buildLogsFile]);

  const [isDownloading, setIsDownloading] = useState(false);
  const isWorkspaceHealthy = workspace.health.healthy;
  const isLoadingFiles = allFiles.some((f) => f.blob === undefined);

  const resetDownloadStateIdRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    const clearTimeoutOnUnmount = () => {
      window.clearTimeout(resetDownloadStateIdRef.current);
    };

    return clearTimeoutOnUnmount;
  }, []);

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
        // they can. Otherwise, wait for everything to come in
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

          resetDownloadStateIdRef.current = window.setTimeout(() => {
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
            <ErrorAlert error="Your workspace is not healthy. Some logs may be unavailable." />
          )}

          <ul css={styles.list}>
            {allFiles.map((f) => (
              <DownloadingItem
                key={f.name}
                file={f}
                giveUpTimeMs={isWorkspaceHealthy ? undefined : 5_000}
              />
            ))}
          </ul>
        </Stack>
      }
    />
  );
};

type DownloadingItemProps = Readonly<{
  // A value of undefined indicates that the component will wait forever
  giveUpTimeMs?: number;
  file: DownloadableFile;
}>;

const DownloadingItem: FC<DownloadingItemProps> = ({ file, giveUpTimeMs }) => {
  const [isWaiting, setIsWaiting] = useState(true);
  useEffect(() => {
    if (giveUpTimeMs === undefined || file.blob !== undefined) {
      setIsWaiting(true);
      return;
    }

    const timeoutId = window.setTimeout(
      () => setIsWaiting(false),
      giveUpTimeMs,
    );

    return () => window.clearTimeout(timeoutId);
  }, [giveUpTimeMs, file]);

  return (
    <li css={styles.listItem}>
      <span css={styles.listItemPrimary}>{file.name}</span>
      <span css={styles.listItemSecondary}>
        {file.blob ? (
          humanBlobSize(file.blob.size)
        ) : isWaiting ? (
          <Skeleton variant="text" width={48} height={12} />
        ) : (
          <p css={styles.notAvailableText}>N/A</p>
        )}
      </span>
    </li>
  );
};

function humanBlobSize(size: number) {
  let i = 0;
  while (size > 1024 && i < BLOB_SIZE_UNITS.length) {
    size /= 1024;
    i++;
  }

  // The while condition can break if we accidentally exceed the bounds of the
  // array. Have to be extra sure we have a unit at the very end.
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

  notAvailableText: (theme) => ({
    color: theme.palette.error.main,
  }),
} satisfies Record<string, Interpolation<Theme>>;
