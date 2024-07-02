import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import Skeleton from "@mui/material/Skeleton";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { type FC, useMemo, useState, useRef, useEffect } from "react";
import { useQueries, useQuery } from "react-query";
import { agentLogs, buildLogs } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
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
  open,
  onClose,
  download = saveAs,
}) => {
  const theme = useTheme();

  const buildLogsQuery = useQuery({
    ...buildLogs(workspace),
    enabled: open,
  });

  const allUniqueAgents = useMemo<readonly WorkspaceAgent[]>(() => {
    const allAgents = workspace.latest_build.resources.flatMap(
      (resource) => resource.agents ?? [],
    );

    // Can't use the "new Set()" trick because we're not dealing with primitives
    const uniqueAgents = new Map(allAgents.map((agent) => [agent.id, agent]));
    const iterable = [...uniqueAgents.values()];
    return iterable;
  }, [workspace.latest_build.resources]);

  const agentLogQueries = useQueries({
    queries: allUniqueAgents.map((agent) => ({
      ...agentLogs(workspace.id, agent.id),
      enabled: open,
    })),
  });

  // Note: trying to memoize this via useMemo got really clunky. Removing all
  // memoization for now, but if we get to a point where performance matters,
  // we should make it so that this state doesn't even begin to mount until the
  // user decides to open the Logs dropdown
  const allFiles = ((): readonly DownloadableFile[] => {
    const files = allUniqueAgents.map<DownloadableFile>((a, i) => {
      const name = `${a.name}-logs.txt`;
      const txt = agentLogQueries[i]?.data?.map((l) => l.output).join("\n");

      let blob: Blob | undefined;
      if (txt) {
        blob = new Blob([txt], { type: "text/plain" });
      }

      return { name, blob };
    });

    const buildLogFile = {
      name: `${workspace.name}-build-logs.txt`,
      blob: buildLogsQuery.data
        ? new Blob([buildLogsQuery.data.map((l) => l.output).join("\n")], {
            type: "text/plain",
          })
        : undefined,
    };

    files.unshift(buildLogFile);
    return files;
  })();

  const [isDownloading, setIsDownloading] = useState(false);
  const isWorkspaceHealthy = workspace.health.healthy;
  const isLoadingFiles = allFiles.some((f) => f.blob === undefined);

  const downloadTimeoutIdRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    const clearTimeoutOnUnmount = () => {
      window.clearTimeout(downloadTimeoutIdRef.current);
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

          downloadTimeoutIdRef.current = window.setTimeout(() => {
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
  const theme = useTheme();
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
      <span
        css={[
          styles.listItemPrimary,
          !isWaiting && { color: theme.palette.text.disabled },
        ]}
      >
        {file.name}
      </span>

      <span css={styles.listItemSecondary}>
        {file.blob ? (
          humanBlobSize(file.blob.size)
        ) : isWaiting ? (
          <Skeleton variant="text" width={48} height={12} />
        ) : (
          <div css={styles.notAvailableText}>
            <span aria-hidden>
              <ErrorIcon fontSize="inherit" />
            </span>

            <p>N/A</p>
          </div>
        )}
      </span>
    </li>
  );
};

function humanBlobSize(size: number) {
  const BLOB_SIZE_UNITS = ["B", "KB", "MB", "GB", "TB"] as const;
  let i = 0;
  while (size > 1024 && i < BLOB_SIZE_UNITS.length) {
    size /= 1024;
    i++;
  }

  // The condition for the while loop above means that over time, we could break
  // out of the loop because we accidentally went out of the array bounds.
  // Adding a lot of redundant checks to make sure we always have a usable unit
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
    display: "flex",
    flexFlow: "row nowrap",
    alignItems: "center",
    columnGap: "4px",

    "& > span": {
      maxHeight: "fit-content",
      display: "flex",
      alignItems: "center",
      color: theme.palette.error.light,
    },

    "& > p": {
      opacity: "80%",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
