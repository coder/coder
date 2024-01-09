import PersonOutlinedIcon from "@mui/icons-material/PersonOutlined";
import ScheduleIcon from "@mui/icons-material/Schedule";
import { visuallyHidden } from "@mui/utils";
import dayjs from "dayjs";
import "dayjs/plugin/relativeTime";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC, type ReactNode, useState } from "react";
import { useMutation } from "react-query";
import { deleteWorkspace, startWorkspace, stopWorkspace } from "api/api";
import type { Workspace } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { getResourceIconPath } from "utils/workspace";
import { Stack } from "components/Stack/Stack";

interface UseBatchActionsProps {
  onSuccess: () => Promise<void>;
}

export function useBatchActions(options: UseBatchActionsProps) {
  const { onSuccess } = options;

  const startAllMutation = useMutation({
    mutationFn: async (workspaces: Workspace[]) => {
      return Promise.all(
        workspaces.map((w) =>
          startWorkspace(w.id, w.latest_build.template_version_id),
        ),
      );
    },
    onSuccess,
    onError: () => {
      displayError("Failed to start workspaces");
    },
  });

  const stopAllMutation = useMutation({
    mutationFn: async (workspaces: Workspace[]) => {
      return Promise.all(workspaces.map((w) => stopWorkspace(w.id)));
    },
    onSuccess,
    onError: () => {
      displayError("Failed to stop workspaces");
    },
  });

  const deleteAllMutation = useMutation({
    mutationFn: async (workspaces: Workspace[]) => {
      return Promise.all(workspaces.map((w) => deleteWorkspace(w.id)));
    },
    onSuccess,
    onError: () => {
      displayError("Failed to delete workspaces");
    },
  });

  return {
    startAll: startAllMutation.mutateAsync,
    stopAll: stopAllMutation.mutateAsync,
    deleteAll: deleteAllMutation.mutateAsync,
    isLoading:
      startAllMutation.isLoading ||
      stopAllMutation.isLoading ||
      deleteAllMutation.isLoading,
  };
}

type BatchDeleteConfirmationProps = {
  checkedWorkspaces: Workspace[];
  open: boolean;
  isLoading: boolean;
  onClose: () => void;
  onConfirm: () => void;
};

export const BatchDeleteConfirmation: FC<BatchDeleteConfirmationProps> = ({
  checkedWorkspaces,
  open,
  onClose,
  onConfirm,
  isLoading,
}) => {
  const [stage, setStage] = useState<
    "consequences" | "workspaces" | "resources"
  >("consequences");

  const onProceed = () => {
    switch (stage) {
      case "resources":
        onConfirm();
        break;
      case "workspaces":
        setStage("resources");
        break;
      case "consequences":
        setStage("workspaces");
        break;
    }
  };

  const workspaceCount = `${checkedWorkspaces.length} ${
    checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
  }`;

  let confirmText: ReactNode = <>Review selected workspaces&hellip;</>;
  if (stage === "workspaces") {
    confirmText = <>Confirm {workspaceCount}&hellip;</>;
  }
  if (stage === "resources") {
    const resources = checkedWorkspaces
      .map((workspace) => workspace.latest_build.resources.length)
      .reduce((a, b) => a + b, 0);
    const resourceCount = `${resources} ${
      resources === 1 ? "resource" : "resources"
    }`;
    confirmText = (
      <>
        Delete {workspaceCount} and {resourceCount}
      </>
    );
  }

  // The flicker of these icons is quit noticeable if they aren't loaded in advance,
  // so we insert them into the document without actually displaying them yet.
  const resourceIconPreloads = [
    ...new Set(
      checkedWorkspaces.flatMap((workspace) =>
        workspace.latest_build.resources.map(
          (resource) => resource.icon || getResourceIconPath(resource.type),
        ),
      ),
    ),
  ].map((url) => (
    <img key={url} alt="" aria-hidden css={{ ...visuallyHidden }} src={url} />
  ));

  return (
    <ConfirmDialog
      type="delete"
      open={open}
      onClose={() => {
        setStage("consequences");
        onClose();
      }}
      title={`Delete ${workspaceCount}`}
      hideCancel
      confirmLoading={isLoading}
      confirmText={confirmText}
      onConfirm={onProceed}
      description={
        <>
          {stage === "consequences" && <Consequences />}
          {stage === "workspaces" && (
            <Workspaces workspaces={checkedWorkspaces} />
          )}
          {stage === "resources" && (
            <Resources workspaces={checkedWorkspaces} />
          )}
          {resourceIconPreloads}
        </>
      }
    />
  );
};

interface StageProps {
  workspaces: Workspace[];
}

const Consequences: FC = () => {
  return (
    <>
      <p>Deleting workspaces is irreversible!</p>
      <ul css={styles.consequences}>
        <li>
          Terraform resources belonging to deleted workspaces will be destroyed.
        </li>
        <li>Any data stored in the workspace will be permanently deleted.</li>
      </ul>
    </>
  );
};

const Workspaces: FC<StageProps> = ({ workspaces }) => {
  const mostRecent = workspaces.reduce(
    (latestSoFar, against) => {
      if (!latestSoFar) {
        return against;
      }

      return new Date(against.last_used_at).getTime() >
        new Date(latestSoFar.last_used_at).getTime()
        ? against
        : latestSoFar;
    },
    undefined as Workspace | undefined,
  );

  const owners = new Set(workspaces.map((it) => it.owner_id)).size;
  const ownersCount = `${owners} ${owners === 1 ? "owner" : "owners"}`;

  return (
    <>
      <ul css={styles.workspacesList}>
        {workspaces.map((workspace) => (
          <li key={workspace.id} css={styles.workspace}>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
            >
              <span css={{ fontWeight: 500, color: "#fff" }}>
                {workspace.name}
              </span>
              <Stack css={{ gap: 0, fontSize: 14, width: 128 }}>
                <Stack direction="row" alignItems="center" spacing={1}>
                  <PersonIcon />
                  <span
                    css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}
                  >
                    {workspace.owner_name}
                  </span>
                </Stack>
                <Stack direction="row" alignItems="center" spacing={1}>
                  <ScheduleIcon css={styles.summaryIcon} />
                  <span
                    css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}
                  >
                    {dayjs(workspace.last_used_at).fromNow()}
                  </span>
                </Stack>
              </Stack>
            </Stack>
          </li>
        ))}
      </ul>
      <Stack justifyContent="center" direction="row" css={{ fontSize: 14 }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <PersonIcon />
          <span>{ownersCount}</span>
        </Stack>
        {mostRecent && (
          <Stack direction="row" alignItems="center" spacing={1}>
            <ScheduleIcon css={styles.summaryIcon} />
            <span>Last used {dayjs(mostRecent.last_used_at).fromNow()}</span>
          </Stack>
        )}
      </Stack>
    </>
  );
};

const Resources: FC<StageProps> = ({ workspaces }) => {
  const resources: Record<string, { count: number; icon: string }> = {};
  workspaces.forEach((workspace) =>
    workspace.latest_build.resources.forEach((resource) => {
      if (!resources[resource.type]) {
        resources[resource.type] = {
          count: 0,
          icon: resource.icon || getResourceIconPath(resource.type),
        };
      }

      resources[resource.type].count++;
    }),
  );

  return (
    <Stack>
      <p>
        Deleting{" "}
        {workspaces.length === 1 ? "this workspace" : "these workspaces"} will
        also permanently destroy&hellip;
      </p>
      <Stack
        direction="row"
        justifyContent="center"
        wrap="wrap"
        css={{ gap: "6px 20px", fontSize: 14 }}
      >
        {Object.entries(resources).map(([type, summary]) => (
          <Stack key={type} direction="row" alignItems="center" spacing={1}>
            <img alt="" src={summary.icon} css={styles.summaryIcon} />
            <span>
              {summary.count} <code>{type}</code>
            </span>
          </Stack>
        ))}
      </Stack>
    </Stack>
  );
};

const PersonIcon: FC = () => {
  // This size doesn't match the rest of the icons because MUI is just really
  // inconsistent. We have to make it bigger than the rest, and pull things in
  // on the sides to compensate.
  return <PersonOutlinedIcon css={{ width: 18, height: 18, margin: -1 }} />;
};

const styles = {
  summaryIcon: { width: 16, height: 16 },

  consequences: {
    display: "flex",
    flexDirection: "column",
    gap: 8,
    paddingLeft: 16,
    marginBottom: 0,
  },

  workspacesList: (theme) => ({
    listStyleType: "none",
    padding: 0,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    overflow: "hidden auto",
    maxHeight: 184,
  }),

  workspace: (theme) => ({
    padding: "8px 16px",
    borderBottom: `1px solid ${theme.palette.divider}`,

    "&:last-child": {
      border: "none",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
