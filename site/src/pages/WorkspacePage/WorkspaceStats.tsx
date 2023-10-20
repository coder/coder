import Link from "@mui/material/Link";
import { WorkspaceOutdatedTooltip } from "components/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import {
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
  isWorkspaceOn,
} from "utils/workspace";
import { Workspace } from "api/typesGenerated";
import { Stats, StatsItem } from "components/Stats/Stats";
import upperFirst from "lodash/upperFirst";
import { autostartDisplay, autostopDisplay } from "utils/schedule";
import IconButton from "@mui/material/IconButton";
import RemoveIcon from "@mui/icons-material/RemoveOutlined";
import { makeStyles } from "@mui/styles";
import AddIcon from "@mui/icons-material/AddOutlined";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import { WorkspaceStatusText } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge";
import { DormantDeletionStat } from "components/WorkspaceDeletion";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  statusLabel: "Workspace Status",
  versionLabel: "Version",
  lastBuiltLabel: "Last built",
  outdated: "Outdated",
  upToDate: "Up to date",
  byLabel: "Last built by",
  costLabel: "Daily cost",
};

export interface WorkspaceStatsProps {
  workspace: Workspace;
  maxDeadlineIncrease: number;
  maxDeadlineDecrease: number;
  canUpdateWorkspace: boolean;
  quotaBudget?: number;
  onDeadlinePlus: (hours: number) => void;
  onDeadlineMinus: (hours: number) => void;
  handleUpdate: () => void;
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  quotaBudget,
  maxDeadlineDecrease,
  maxDeadlineIncrease,
  canUpdateWorkspace,
  handleUpdate,
  onDeadlineMinus,
  onDeadlinePlus,
}) => {
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(
    workspace.latest_build,
  );
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace);
  const styles = useStyles();
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1;
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1;

  return (
    <>
      <Stats aria-label={Language.workspaceDetails} className={styles.stats}>
        <StatsItem
          className={styles.statsItem}
          label="Status"
          value={<WorkspaceStatusText workspace={workspace} />}
        />
        <DormantDeletionStat workspace={workspace} />
        <StatsItem
          className={styles.statsItem}
          label={Language.templateLabel}
          value={
            <Link
              component={RouterLink}
              to={`/templates/${workspace.template_name}`}
            >
              {displayTemplateName}
            </Link>
          }
        />
        <StatsItem
          className={styles.statsItem}
          label={Language.versionLabel}
          value={
            <>
              <Link
                component={RouterLink}
                to={`/templates/${workspace.template_name}/versions/${workspace.latest_build.template_version_name}`}
              >
                {workspace.latest_build.template_version_name}
              </Link>

              {workspace.outdated && (
                <WorkspaceOutdatedTooltip
                  templateName={workspace.template_name}
                  latestVersionId={workspace.template_active_version_id}
                  onUpdateVersion={handleUpdate}
                  ariaLabel="update version"
                />
              )}
            </>
          }
        />
        <StatsItem
          className={styles.statsItem}
          label={Language.lastBuiltLabel}
          value={
            <>
              {upperFirst(createDayString(workspace.latest_build.created_at))}{" "}
              by {initiatedBy}
            </>
          }
        />
        {shouldDisplayScheduleLabel(workspace) && (
          <StatsItem
            className={styles.statsItem}
            label={getScheduleLabel(workspace)}
            value={
              <span className={styles.scheduleValue}>
                <Link
                  component={RouterLink}
                  to="settings/schedule"
                  title="Schedule settings"
                >
                  {isWorkspaceOn(workspace)
                    ? autostopDisplay(workspace)
                    : autostartDisplay(workspace.autostart_schedule)}
                </Link>
                {canUpdateWorkspace && canEditDeadline(workspace) && (
                  <span className={styles.scheduleControls}>
                    <Popover>
                      <PopoverTrigger>
                        <IconButton
                          disabled={!deadlineMinusEnabled}
                          size="small"
                          title="Subtract hours from deadline"
                          className={styles.scheduleButton}
                        >
                          <RemoveIcon />
                        </IconButton>
                      </PopoverTrigger>
                      <PopoverContent
                        id="schedule-sub"
                        classes={{ paper: styles.timePopoverPaper }}
                        horizontal="right"
                      >
                        <DecreaseTimeContent
                          maxDeadlineDecrease={maxDeadlineDecrease}
                          onDeadlineMinus={onDeadlineMinus}
                        />
                      </PopoverContent>
                    </Popover>
                    <Popover>
                      <PopoverTrigger>
                        <IconButton
                          disabled={!deadlinePlusEnabled}
                          size="small"
                          title="Add hours to deadline"
                          className={styles.scheduleButton}
                        >
                          <AddIcon />
                        </IconButton>
                      </PopoverTrigger>
                      <PopoverContent
                        id="schedule-add"
                        classes={{ paper: styles.timePopoverPaper }}
                        horizontal="right"
                      >
                        <AddTimeContent
                          maxDeadlineIncrease={maxDeadlineIncrease}
                          onDeadlinePlus={onDeadlinePlus}
                        />
                      </PopoverContent>
                    </Popover>
                  </span>
                )}
              </span>
            }
          />
        )}
        {workspace.latest_build.daily_cost > 0 && (
          <StatsItem
            className={styles.statsItem}
            label={Language.costLabel}
            value={`${workspace.latest_build.daily_cost} ${
              quotaBudget ? `/ ${quotaBudget}` : ""
            }`}
          />
        )}
      </Stats>
    </>
  );
};

const AddTimeContent = (props: {
  maxDeadlineIncrease: number;
  onDeadlinePlus: (value: number) => void;
}) => {
  const styles = useStyles();
  const popover = usePopover();

  return (
    <>
      <span className={styles.timePopoverTitle}>Add hours to deadline</span>
      <span className={styles.timePopoverDescription}>
        Delay the shutdown of this workspace for a few more hours. This is only
        applied once.
      </span>
      <form
        className={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          props.onDeadlinePlus(hours);
          popover.setIsOpen(false);
        }}
      >
        <TextField
          name="hours"
          type="number"
          size="small"
          fullWidth
          className={styles.timePopoverField}
          InputProps={{
            className: styles.timePopoverFieldInput,
          }}
          inputProps={{
            min: 0,
            max: props.maxDeadlineIncrease,
            step: 1,
            defaultValue: 1,
          }}
        />

        <Button className={styles.timePopoverButton} type="submit">
          Apply
        </Button>
      </form>
    </>
  );
};

export const DecreaseTimeContent = (props: {
  onDeadlineMinus: (hours: number) => void;
  maxDeadlineDecrease: number;
}) => {
  const styles = useStyles();
  const popover = usePopover();

  return (
    <>
      <span className={styles.timePopoverTitle}>
        Subtract hours to deadline
      </span>
      <span className={styles.timePopoverDescription}>
        Anticipate the shutdown of this workspace for a few more hours. This is
        only applied once.
      </span>
      <form
        className={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          props.onDeadlineMinus(hours);
          popover.setIsOpen(false);
        }}
      >
        <TextField
          name="hours"
          type="number"
          size="small"
          fullWidth
          className={styles.timePopoverField}
          InputProps={{
            className: styles.timePopoverFieldInput,
          }}
          inputProps={{
            min: 0,
            max: props.maxDeadlineDecrease,
            step: 1,
            defaultValue: 1,
          }}
        />

        <Button className={styles.timePopoverButton} type="submit">
          Apply
        </Button>
      </form>
    </>
  );
};

export const canEditDeadline = (workspace: Workspace): boolean => {
  return isWorkspaceOn(workspace) && Boolean(workspace.latest_build.deadline);
};

export const shouldDisplayScheduleLabel = (workspace: Workspace): boolean => {
  if (canEditDeadline(workspace)) {
    return true;
  }
  if (isWorkspaceOn(workspace)) {
    return false;
  }
  return Boolean(workspace.autostart_schedule);
};

const getScheduleLabel = (workspace: Workspace) => {
  return isWorkspaceOn(workspace) ? "Stops at" : "Starts at";
};

const useStyles = makeStyles((theme) => ({
  stats: {
    padding: 0,
    border: 0,
    gap: theme.spacing(6),
    rowGap: theme.spacing(3),
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: theme.spacing(1),
    },
  },

  statsItem: {
    flexDirection: "column",
    gap: 0,
    padding: 0,

    "& > span:first-of-type": {
      fontSize: 12,
      fontWeight: 500,
    },
  },

  scheduleValue: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1.5),
  },

  scheduleControls: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },

  scheduleButton: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,
    width: 20,
    height: 20,

    "& svg.MuiSvgIcon-root": {
      width: theme.spacing(1.5),
      height: theme.spacing(1.5),
    },
  },

  timePopoverPaper: {
    padding: theme.spacing(3),
    maxWidth: theme.spacing(36),
    marginTop: theme.spacing(1),
    borderRadius: 4,
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(1),
  },

  timePopoverTitle: {
    fontWeight: 600,
  },

  timePopoverDescription: {
    color: theme.palette.text.secondary,
  },

  timePopoverForm: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
    padding: theme.spacing(1, 0),
  },

  timePopoverField: {
    margin: 0,
  },

  timePopoverFieldInput: {
    fontSize: 14,
    padding: theme.spacing(0),
    borderRadius: 4,
  },

  timePopoverButton: {
    borderRadius: 4,
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    flexShrink: 0,
  },
}));
