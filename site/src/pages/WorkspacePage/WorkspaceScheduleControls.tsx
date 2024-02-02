import { type Interpolation, type Theme } from "@emotion/react";
import Link, { LinkProps } from "@mui/material/Link";
import { forwardRef, type FC, useRef } from "react";
import { Link as RouterLink } from "react-router-dom";
import { isWorkspaceOn } from "utils/workspace";
import type { Workspace } from "api/typesGenerated";
import {
  autostartDisplay,
  autostopDisplay,
  getDeadline,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
} from "utils/schedule";
import IconButton from "@mui/material/IconButton";
import RemoveIcon from "@mui/icons-material/RemoveOutlined";
import AddIcon from "@mui/icons-material/AddOutlined";
import Tooltip from "@mui/material/Tooltip";
import _ from "lodash";
import { getErrorMessage } from "api/errors";
import {
  updateDeadline,
  workspaceByOwnerAndNameKey,
} from "api/queries/workspaces";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useMutation, useQueryClient } from "react-query";
import { Dayjs } from "dayjs";
import { visuallyHidden } from "@mui/utils";

export interface WorkspaceScheduleControlsProps {
  workspace: Workspace;
  canUpdateSchedule: boolean;
}

export const WorkspaceScheduleControls: FC<WorkspaceScheduleControlsProps> = ({
  workspace,
  canUpdateSchedule,
}) => {
  const queryClient = useQueryClient();
  const deadline = getDeadline(workspace);
  const maxDeadlineDecrease = getMaxDeadlineChange(deadline, getMinDeadline());
  const maxDeadlineIncrease = getMaxDeadlineChange(
    getMaxDeadline(workspace),
    deadline,
  );
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1;
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1;
  const deadlineUpdateTimeout = useRef<number>();
  const lastStableDeadline = useRef<Dayjs>(deadline);

  const updateWorkspaceDeadlineQueryData = (deadline: Dayjs) => {
    queryClient.setQueryData(
      workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
      {
        ...workspace,
        latest_build: {
          ...workspace.latest_build,
          deadline: deadline.toISOString(),
        },
      },
    );
  };

  const updateDeadlineMutation = useMutation({
    ...updateDeadline(workspace),
    onSuccess: (_, updatedDeadline) => {
      displaySuccess("Workspace shutdown time has been successfully updated.");
      lastStableDeadline.current = updatedDeadline;
    },
    onError: (error) => {
      displayError(
        getErrorMessage(
          error,
          "We couldn't update your workspace shutdown time. Please try again.",
        ),
      );
      updateWorkspaceDeadlineQueryData(lastStableDeadline.current);
    },
  });

  const handleDeadlineChange = (newDeadline: Dayjs) => {
    clearTimeout(deadlineUpdateTimeout.current);
    // Optimistic update
    updateWorkspaceDeadlineQueryData(newDeadline);
    deadlineUpdateTimeout.current = window.setTimeout(() => {
      updateDeadlineMutation.mutate(newDeadline);
    }, 500);
  };

  return (
    <div css={styles.scheduleValue} data-testid="schedule-controls">
      {isWorkspaceOn(workspace) ? (
        <AutoStopDisplay workspace={workspace} />
      ) : (
        <ScheduleSettingsLink>
          Starts at {autostartDisplay(workspace.autostart_schedule)}
        </ScheduleSettingsLink>
      )}

      {canUpdateSchedule && canEditDeadline(workspace) && (
        <div css={styles.scheduleControls}>
          <Tooltip title="Subtract 1 hour from deadline">
            <IconButton
              disabled={!deadlineMinusEnabled}
              size="small"
              css={styles.scheduleButton}
              onClick={() => {
                handleDeadlineChange(deadline.subtract(1, "h"));
              }}
            >
              <RemoveIcon />
              <span style={visuallyHidden}>Subtract 1 hour</span>
            </IconButton>
          </Tooltip>
          <Tooltip title="Add 1 hour to deadline">
            <IconButton
              disabled={!deadlinePlusEnabled}
              size="small"
              css={styles.scheduleButton}
              onClick={() => {
                handleDeadlineChange(deadline.add(1, "h"));
              }}
            >
              <AddIcon />
              <span style={visuallyHidden}>Add 1 hour</span>
            </IconButton>
          </Tooltip>
        </div>
      )}
    </div>
  );
};

interface AutoStopDisplayProps {
  workspace: Workspace;
}

const AutoStopDisplay: FC<AutoStopDisplayProps> = ({ workspace }) => {
  const display = autostopDisplay(workspace);

  if (display.tooltip) {
    return (
      <Tooltip title={display.tooltip}>
        <ScheduleSettingsLink
          css={(theme) => ({
            color: isShutdownSoon(workspace)
              ? `${theme.palette.warning.light} !important`
              : undefined,
          })}
        >
          Stop {display.message}
        </ScheduleSettingsLink>
      </Tooltip>
    );
  }

  return <ScheduleSettingsLink>{display.message}</ScheduleSettingsLink>;
};

const ScheduleSettingsLink = forwardRef<HTMLAnchorElement, LinkProps>(
  (props, ref) => {
    return (
      <Link
        ref={ref}
        component={RouterLink}
        to="settings/schedule"
        css={{
          color: "inherit",
          "&:first-letter": {
            textTransform: "uppercase",
          },
        }}
        {...props}
      />
    );
  },
);

const hasDeadline = (workspace: Workspace): boolean => {
  return Boolean(workspace.latest_build.deadline);
};

const hasAutoStart = (workspace: Workspace): boolean => {
  return Boolean(workspace.autostart_schedule);
};

export const canEditDeadline = (workspace: Workspace): boolean => {
  return isWorkspaceOn(workspace) && hasDeadline(workspace);
};

export const shouldDisplayScheduleControls = (
  workspace: Workspace,
): boolean => {
  const willAutoStop = isWorkspaceOn(workspace) && hasDeadline(workspace);
  const willAutoStart = !isWorkspaceOn(workspace) && hasAutoStart(workspace);
  return willAutoStop || willAutoStart;
};

const isShutdownSoon = (workspace: Workspace): boolean => {
  const deadline = workspace.latest_build.deadline;
  if (!deadline) {
    return false;
  }
  const deadlineDate = new Date(deadline);
  const now = new Date();
  const diff = deadlineDate.getTime() - now.getTime();
  const oneHour = 1000 * 60 * 60;
  return diff < oneHour;
};

const styles = {
  scheduleValue: {
    display: "flex",
    alignItems: "center",
    gap: 12,
    fontVariantNumeric: "tabular-nums",
  },

  scheduleControls: {
    display: "flex",
    alignItems: "center",
    gap: 4,
  },

  scheduleButton: (theme) => ({
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,
    width: 20,
    height: 20,

    "& svg.MuiSvgIcon-root": {
      width: 12,
      height: 12,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
