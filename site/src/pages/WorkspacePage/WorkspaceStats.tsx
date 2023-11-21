import { css } from "@emotion/css";
import { type Interpolation, type Theme } from "@emotion/react";
import Link, { LinkProps } from "@mui/material/Link";
import { WorkspaceOutdatedTooltip } from "components/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { forwardRef, type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import {
  getDisplayWorkspaceTemplateName,
  isWorkspaceOn,
} from "utils/workspace";
import type { Workspace } from "api/typesGenerated";
import { Stats, StatsItem } from "components/Stats/Stats";
import { autostartDisplay, autostopDisplay } from "utils/schedule";
import IconButton from "@mui/material/IconButton";
import RemoveIcon from "@mui/icons-material/RemoveOutlined";
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
import { workspaceQuota } from "api/queries/workspaceQuota";
import { useQuery } from "react-query";
import Tooltip from "@mui/material/Tooltip";
import _ from "lodash";

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  costLabel: "Daily cost",
  updatePolicy: "Update policy",
};

export interface WorkspaceStatsProps {
  workspace: Workspace;
  maxDeadlineIncrease: number;
  maxDeadlineDecrease: number;
  canUpdateWorkspace: boolean;
  onDeadlinePlus: (hours: number) => void;
  onDeadlineMinus: (hours: number) => void;
  handleUpdate: () => void;
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  maxDeadlineDecrease,
  maxDeadlineIncrease,
  canUpdateWorkspace,
  handleUpdate,
  onDeadlineMinus,
  onDeadlinePlus,
}) => {
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace);
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1;
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1;
  const quotaQuery = useQuery(workspaceQuota(workspace.owner_name));
  const quotaBudget = quotaQuery.data?.budget;

  const paperStyles = css`
    padding: 24px;
    max-width: 288px;
    margin-top: ${8};
    border-radius: 4px;
    display: flex;
    flex-direction: column;
    gap: ${8};
  `;

  return (
    <>
      <Stats aria-label={Language.workspaceDetails} css={styles.stats}>
        <StatsItem
          css={styles.statsItem}
          label="Status"
          value={<WorkspaceStatusText workspace={workspace} />}
        />
        <DormantDeletionStat workspace={workspace} />
        <StatsItem
          css={styles.statsItem}
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
          css={styles.statsItem}
          label="Version"
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

        {shouldDisplayScheduleLabel(workspace) && (
          <StatsItem
            css={styles.statsItem}
            label={getScheduleLabel(workspace)}
            value={
              <div css={styles.scheduleValue}>
                {isWorkspaceOn(workspace) ? (
                  <AutoStopDisplay workspace={workspace} />
                ) : (
                  <ScheduleSettingsLink>
                    {autostartDisplay(workspace.autostart_schedule)}
                  </ScheduleSettingsLink>
                )}

                {canUpdateWorkspace && canEditDeadline(workspace) && (
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
                        classes={{ paper: paperStyles }}
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
                          css={styles.scheduleButton}
                        >
                          <AddIcon />
                        </IconButton>
                      </PopoverTrigger>
                      <PopoverContent
                        id="schedule-add"
                        classes={{ paper: paperStyles }}
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
              </div>
            }
          />
        )}
        {workspace.latest_build.daily_cost > 0 && (
          <StatsItem
            css={styles.statsItem}
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
  const popover = usePopover();

  return (
    <>
      <span css={styles.timePopoverTitle}>Add hours to deadline</span>
      <span css={styles.timePopoverDescription}>
        Delay the shutdown of this workspace for a few more hours. This is only
        applied once.
      </span>
      <form
        css={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          props.onDeadlinePlus(hours);
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
          InputProps={{
            className: timePopoverFieldInputStyles,
          }}
          inputProps={{
            min: 0,
            max: props.maxDeadlineIncrease,
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

export const DecreaseTimeContent = (props: {
  onDeadlineMinus: (hours: number) => void;
  maxDeadlineDecrease: number;
}) => {
  const popover = usePopover();

  return (
    <>
      <span css={styles.timePopoverTitle}>Subtract hours to deadline</span>
      <span css={styles.timePopoverDescription}>
        Anticipate the shutdown of this workspace for a few more hours. This is
        only applied once.
      </span>
      <form
        css={styles.timePopoverForm}
        onSubmit={(e) => {
          e.preventDefault();
          const formData = new FormData(e.currentTarget);
          const hours = Number(formData.get("hours"));
          props.onDeadlineMinus(hours);
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
          InputProps={{
            className: timePopoverFieldInputStyles,
          }}
          inputProps={{
            min: 0,
            max: props.maxDeadlineDecrease,
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

const AutoStopDisplay = (props: { workspace: Workspace }) => {
  const { workspace } = props;
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
  return isWorkspaceOn(workspace) ? "Stops" : "Starts at";
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

const timePopoverFieldInputStyles = css`
  font-size: 14px;
  padding: 0px;
  border-radius: 4px;
`;

const styles = {
  stats: (theme) => ({
    padding: 0,
    border: 0,
    gap: 48,
    rowGap: 24,
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: 8,
    },
  }),

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
    marginBottom: 8,
  },

  timePopoverDescription: (theme) => ({
    color: theme.palette.text.secondary,
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
