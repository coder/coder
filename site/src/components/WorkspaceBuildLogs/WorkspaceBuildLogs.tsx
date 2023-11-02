import dayjs from "dayjs";
import { type ComponentProps, type FC, Fragment } from "react";
import type { ProvisionerJobLog } from "api/typesGenerated";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { Logs } from "./Logs";
import Box from "@mui/material/Box";
import { type Interpolation, type Theme } from "@emotion/react";

const Language = {
  seconds: "seconds",
};

type Stage = ProvisionerJobLog["stage"];
type LogsGroupedByStage = Record<Stage, ProvisionerJobLog[]>;
type GroupLogsByStageFn = (logs: ProvisionerJobLog[]) => LogsGroupedByStage;

export const groupLogsByStage: GroupLogsByStageFn = (logs) => {
  const logsByStage: LogsGroupedByStage = {};

  for (const log of logs) {
    if (log.stage in logsByStage) {
      logsByStage[log.stage].push(log);
    } else {
      logsByStage[log.stage] = [log];
    }
  }

  return logsByStage;
};

const getStageDurationInSeconds = (logs: ProvisionerJobLog[]) => {
  if (logs.length < 2) {
    return;
  }

  const startedAt = dayjs(logs[0].created_at);
  const completedAt = dayjs(logs[logs.length - 1].created_at);
  return completedAt.diff(startedAt, "seconds");
};

export type WorkspaceBuildLogsProps = {
  logs: ProvisionerJobLog[];
  sticky?: boolean;
  hideTimestamps?: boolean;
} & ComponentProps<typeof Box>;

export const WorkspaceBuildLogs: FC<WorkspaceBuildLogsProps> = ({
  hideTimestamps,
  sticky,
  logs,
  ...boxProps
}) => {
  const groupedLogsByStage = groupLogsByStage(logs);
  const stages = Object.keys(groupedLogsByStage);

  return (
    <Box
      {...boxProps}
      sx={{
        border: (theme) => `1px solid ${theme.palette.divider}`,
        borderRadius: 1,
        fontFamily: MONOSPACE_FONT_FAMILY,
        ...boxProps.sx,
      }}
    >
      {stages.map((stage) => {
        const logs = groupedLogsByStage[stage];
        const isEmpty = logs.every((log) => log.output === "");
        const lines = logs.map((log) => ({
          time: log.created_at,
          output: log.output,
          level: log.log_level,
          source_id: log.log_source,
        }));
        const duration = getStageDurationInSeconds(logs);
        const shouldDisplayDuration = duration !== undefined;

        return (
          <Fragment key={stage}>
            <div css={[styles.header, sticky && styles.sticky]}>
              <div>{stage}</div>
              {shouldDisplayDuration && (
                <div css={styles.duration}>
                  {duration} {Language.seconds}
                </div>
              )}
            </div>
            {!isEmpty && <Logs hideTimestamps={hideTimestamps} lines={lines} />}
          </Fragment>
        );
      })}
    </Box>
  );
};

const styles = {
  header: (theme) => ({
    fontSize: 13,
    fontWeight: 600,
    padding: theme.spacing(0.5, 3),
    display: "flex",
    alignItems: "center",
    fontFamily: "Inter",
    borderBottom: `1px solid ${theme.palette.divider}`,
    background: theme.palette.background.default,

    "&:last-child": {
      borderBottom: 0,
      borderRadius: "0 0 8px 8px",
    },

    "&:first-child": {
      borderRadius: "8px 8px 0 0",
    },
  }),

  sticky: {
    position: "sticky",
    top: 0,
  },

  duration: (theme) => ({
    marginLeft: "auto",
    color: theme.palette.text.secondary,
    fontSize: 12,
  }),
} satisfies Record<string, Interpolation<Theme>>;
