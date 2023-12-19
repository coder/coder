import { css } from "@emotion/css";
import { type Interpolation, type Theme } from "@emotion/react";
import Link, { LinkProps } from "@mui/material/Link";
import { forwardRef, type FC } from "react";
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
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";
import Tooltip from "@mui/material/Tooltip";
import _ from "lodash";
import { getErrorMessage } from "api/errors";
import { decreaseDeadline, increaseDeadline } from "api/queries/workspaces";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { useMutation } from "react-query";

export interface WorkspaceScheduleControlsProps {
  workspace: Workspace;
  canUpdateSchedule: boolean;
}

export const WorkspaceScheduleControls: FC<WorkspaceScheduleControlsProps> = ({
  workspace,
  canUpdateSchedule,
}) => {
  const deadline = getDeadline(workspace);
  const maxDeadlineDecrease = getMaxDeadlineChange(deadline, getMinDeadline());
  const maxDeadlineIncrease = getMaxDeadlineChange(
    getMaxDeadline(workspace),
    deadline,
  );
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1;
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1;

  const onDeadlineChangeSuccess = () => {
    displaySuccess("Updated workspace shutdown time.");
  };
  const onDeadlineChangeFails = (error: unknown) => {
    displayError(
      getErrorMessage(error, "Failed to update workspace shutdown time."),
    );
  };
  const decreaseMutation = useMutation({
    ...decreaseDeadline(workspace),
    onSuccess: onDeadlineChangeSuccess,
    onError: onDeadlineChangeFails,
  });
  const increaseMutation = useMutation({
    ...increaseDeadline(workspace),
    onSuccess: onDeadlineChangeSuccess,
    onError: onDeadlineChangeFails,
  });

  return (
    <div css={styles.scheduleValue}>
      {isWorkspaceOn(workspace) ? (
        <AutoStopDisplay workspace={workspace} />
      ) : (
        <ScheduleSettingsLink>
          {autostartDisplay(workspace.autostart_schedule)}
        </ScheduleSettingsLink>
      )}

      {canUpdateSchedule && canEditDeadline(workspace) && (
        <span css={styles.scheduleControls}>
          <Popover>
            <PopoverTrigger>
              <IconButton
                disabled={!deadlineMinusEnabled}
                size="small"
                title="Subtract hours from deadline"
                css={styles.scheduleButton}
              >
                <RemoveIcon />
              </IconButton>
            </PopoverTrigger>
            <PopoverContent
              id="schedule-sub"
              classes={{ paper: classNames.paper }}
              horizontal="right"
            >
              <DecreaseTimeContent
                maxDeadlineDecrease={maxDeadlineDecrease}
                onDeadlineMinus={decreaseMutation.mutate}
              />
            </PopoverContent>
          </Popover>
          <Popover>
            <PopoverTrigger>
              <IconButton
                disabled={!deadlinePlusEnabled}
                size="small"
                title="Add hours to deadline"
                css={styles.scheduleButton}
              >
                <AddIcon />
              </IconButton>
            </PopoverTrigger>
            <PopoverContent
              id="schedule-add"
              classes={{ paper: classNames.paper }}
              horizontal="right"
            >
              <AddTimeContent
                maxDeadlineIncrease={maxDeadlineIncrease}
                onDeadlinePlus={increaseMutation.mutate}
              />
            </PopoverContent>
          </Popover>
        </span>
      )}
    </div>
  );
};

interface AddTimeContentProps {
  maxDeadlineIncrease: number;
  onDeadlinePlus: (value: number) => void;
}

const AddTimeContent: FC<AddTimeContentProps> = ({
  maxDeadlineIncrease,
  onDeadlinePlus,
}) => {
  const popover = usePopover();

  return (
    <>
      <h4 css={styles.timePopoverTitle}>Add hours to deadline</h4>
      <p css={styles.timePopoverDescription}>
        Delay the shutdown of this workspace for a few more hours. This is only
        applied once.
      </p>
      <form
        css={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          onDeadlinePlus(hours);
          popover.setIsOpen(false);
        }}
      >
        <TextField
          autoFocus
          name="hours"
          type="number"
          size="small"
          fullWidth
          css={styles.timePopoverField}
          InputProps={{ className: classNames.deadlineFormInput }}
          inputProps={{
            min: 0,
            max: maxDeadlineIncrease,
            step: 1,
            defaultValue: 1,
          }}
        />

        <Button css={styles.timePopoverButton} type="submit">
          Apply
        </Button>
      </form>
    </>
  );
};

interface DecreaseTimeContentProps {
  maxDeadlineDecrease: number;
  onDeadlineMinus: (hours: number) => void;
}

export const DecreaseTimeContent: FC<DecreaseTimeContentProps> = ({
  maxDeadlineDecrease,
  onDeadlineMinus,
}) => {
  const popover = usePopover();

  return (
    <>
      <h4 css={styles.timePopoverTitle}>Subtract hours to deadline</h4>
      <p css={styles.timePopoverDescription}>
        Anticipate the shutdown of this workspace for a few more hours. This is
        only applied once.
      </p>
      <form
        css={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          onDeadlineMinus(hours);
          popover.setIsOpen(false);
        }}
      >
        <TextField
          autoFocus
          name="hours"
          type="number"
          size="small"
          fullWidth
          css={styles.timePopoverField}
          InputProps={{ className: classNames.deadlineFormInput }}
          inputProps={{
            min: 0,
            max: maxDeadlineDecrease,
            step: 1,
            defaultValue: 1,
          }}
        />

        <Button css={styles.timePopoverButton} type="submit">
          Apply
        </Button>
      </form>
    </>
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
          {display.message}
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

export const scheduleLabel = (workspace: Workspace) => {
  return isWorkspaceOn(workspace) ? "Stops" : "Starts at";
};

const classNames = {
  paper: css`
    padding: 24px;
    max-width: 288px;
    margin-top: 8px;
    border-radius: 4px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  `,

  deadlineFormInput: css`
    font-size: 14px;
    padding: 0px;
    border-radius: 4px;
  `,
};

const styles = {
  scheduleValue: {
    display: "flex",
    alignItems: "center",
    gap: 12,
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

  timePopoverTitle: {
    fontWeight: 600,
    margin: 0,
    marginBottom: 8,
  },

  timePopoverDescription: (theme) => ({
    color: theme.palette.text.secondary,
    margin: 0,
  }),

  timePopoverForm: {
    display: "flex",
    alignItems: "center",
    gap: 8,
    padding: "8px 0",
    marginTop: 12,
  },

  timePopoverField: {
    margin: 0,
  },

  timePopoverButton: {
    borderRadius: 4,
    paddingLeft: 16,
    paddingRight: 16,
    flexShrink: 0,
  },
} satisfies Record<string, Interpolation<Theme>>;
