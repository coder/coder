import Collapse from "@mui/material/Collapse";
import { makeStyles } from "@mui/styles";
import TableCell from "@mui/material/TableCell";
import { AuditLog } from "api/typesGenerated";
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useState } from "react";
import userAgentParser from "ua-parser-js";
import { AuditLogDiff } from "./AuditLogDiff/AuditLogDiff";
import { AuditLogDescription } from "./AuditLogDescription/AuditLogDescription";
import { PaletteIndex } from "theme/theme";
import { determineGroupDiff } from "./AuditLogDiff/auditUtils";

const httpStatusColor = (httpStatus: number): PaletteIndex => {
  // redirects are successful
  if (httpStatus === 307) {
    return "success";
  }

  if (httpStatus >= 300 && httpStatus < 500) {
    return "warning";
  }

  if (httpStatus >= 500) {
    return "error";
  }

  return "success";
};

export interface AuditLogRowProps {
  auditLog: AuditLog;
  // Useful for Storybook
  defaultIsDiffOpen?: boolean;
}

export const AuditLogRow: React.FC<AuditLogRowProps> = ({
  auditLog,
  defaultIsDiffOpen = false,
}) => {
  const styles = useStyles();
  const [isDiffOpen, setIsDiffOpen] = useState(defaultIsDiffOpen);
  const diffs = Object.entries(auditLog.diff);
  const shouldDisplayDiff = diffs.length > 0;
  const { os, browser } = userAgentParser(auditLog.user_agent);

  let auditDiff = auditLog.diff;

  // groups have nested diffs (group members)
  if (auditLog.resource_type === "group") {
    auditDiff = determineGroupDiff(auditLog.diff);
  }

  const toggle = () => {
    if (shouldDisplayDiff) {
      setIsDiffOpen((v) => !v);
    }
  };

  return (
    <TimelineEntry
      key={auditLog.id}
      data-testid={`audit-log-row-${auditLog.id}`}
      clickable={shouldDisplayDiff}
    >
      <TableCell className={styles.auditLogCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.auditLogHeader}
          tabIndex={0}
          onClick={toggle}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              toggle();
            }
          }}
        >
          <Stack
            direction="row"
            alignItems="center"
            className={styles.auditLogHeaderInfo}
          >
            <Stack
              direction="row"
              alignItems="center"
              className={styles.fullWidth}
            >
              <UserAvatar
                username={auditLog.user?.username ?? "?"}
                avatarURL={auditLog.user?.avatar_url}
              />

              <Stack
                alignItems="baseline"
                className={styles.fullWidth}
                justifyContent="space-between"
                direction="row"
              >
                <Stack
                  className={styles.auditLogSummary}
                  direction="row"
                  alignItems="baseline"
                  spacing={1}
                >
                  <AuditLogDescription auditLog={auditLog} />
                  {auditLog.is_deleted && (
                    <span className={styles.deletedLabel}>
                      <>(deleted)</>
                    </span>
                  )}
                  <span className={styles.auditLogTime}>
                    {new Date(auditLog.time).toLocaleTimeString()}
                  </span>
                </Stack>

                <Stack direction="row" alignItems="center">
                  <Stack direction="row" spacing={1} alignItems="baseline">
                    {auditLog.ip && (
                      <span className={styles.auditLogInfo}>
                        <>IP: </>
                        <strong>{auditLog.ip}</strong>
                      </span>
                    )}
                    {os.name && (
                      <span className={styles.auditLogInfo}>
                        <>OS: </>
                        <strong>{os.name}</strong>
                      </span>
                    )}
                    {browser.name && (
                      <span className={styles.auditLogInfo}>
                        <>Browser: </>
                        <strong>
                          {browser.name} {browser.version}
                        </strong>
                      </span>
                    )}
                  </Stack>

                  <Pill
                    className={styles.httpStatusPill}
                    type={httpStatusColor(auditLog.status_code)}
                    text={auditLog.status_code.toString()}
                  />
                </Stack>
              </Stack>
            </Stack>
          </Stack>

          {shouldDisplayDiff ? (
            <div> {isDiffOpen ? <CloseDropdown /> : <OpenDropdown />}</div>
          ) : (
            <div className={styles.columnWithoutDiff}></div>
          )}
        </Stack>

        {shouldDisplayDiff && (
          <Collapse in={isDiffOpen}>
            <AuditLogDiff diff={auditDiff} />
          </Collapse>
        )}
      </TableCell>
    </TimelineEntry>
  );
};

const useStyles = makeStyles((theme) => ({
  auditLogCell: {
    padding: "0 !important",
    border: 0,
  },

  auditLogHeader: {
    padding: theme.spacing(2, 4),
  },

  auditLogHeaderInfo: {
    flex: 1,
  },

  auditLogSummary: {
    ...theme.typography.body1,
    fontFamily: "inherit",
  },

  auditLogTime: {
    color: theme.palette.text.secondary,
    fontSize: 12,
  },

  auditLogInfo: {
    ...theme.typography.body2,
    fontSize: 12,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  },

  // offset the absence of the arrow icon on diff-less logs
  columnWithoutDiff: {
    marginLeft: "24px",
  },

  fullWidth: {
    width: "100%",
  },

  httpStatusPill: {
    fontSize: 10,
    height: 20,
    paddingLeft: 10,
    paddingRight: 10,
    fontWeight: 600,
  },

  deletedLabel: {
    ...theme.typography.caption,
    color: theme.palette.text.secondary,
  },
}));
