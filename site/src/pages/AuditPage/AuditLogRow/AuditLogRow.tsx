import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import Collapse from "@mui/material/Collapse";
import TableCell from "@mui/material/TableCell";
import type { AuditLog } from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Pill, type PillType } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useState } from "react";
import userAgentParser from "ua-parser-js";
import { AuditLogDiff } from "./AuditLogDiff/AuditLogDiff";
import { AuditLogDescription } from "./AuditLogDescription/AuditLogDescription";
import { determineGroupDiff } from "./AuditLogDiff/auditUtils";

const httpStatusColor = (httpStatus: number): PillType => {
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
      <TableCell css={styles.auditLogCell}>
        <Stack
          direction="row"
          alignItems="center"
          css={styles.auditLogHeader}
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
            css={styles.auditLogHeaderInfo}
          >
            <Stack direction="row" alignItems="center" css={styles.fullWidth}>
              <UserAvatar
                username={auditLog.user?.username ?? "?"}
                avatarURL={auditLog.user?.avatar_url}
              />

              <Stack
                alignItems="baseline"
                css={styles.fullWidth}
                justifyContent="space-between"
                direction="row"
              >
                <Stack
                  css={styles.auditLogSummary}
                  direction="row"
                  alignItems="baseline"
                  spacing={1}
                >
                  <AuditLogDescription auditLog={auditLog} />
                  {auditLog.is_deleted && (
                    <span css={styles.deletedLabel}>
                      <>(deleted)</>
                    </span>
                  )}
                  <span css={styles.auditLogTime}>
                    {new Date(auditLog.time).toLocaleTimeString()}
                  </span>
                </Stack>

                <Stack direction="row" alignItems="center">
                  <Stack direction="row" spacing={1} alignItems="baseline">
                    {auditLog.ip && (
                      <span css={styles.auditLogInfo}>
                        <>IP: </>
                        <strong>{auditLog.ip}</strong>
                      </span>
                    )}
                    {os.name && (
                      <span css={styles.auditLogInfo}>
                        <>OS: </>
                        <strong>{os.name}</strong>
                      </span>
                    )}
                    {browser.name && (
                      <span css={styles.auditLogInfo}>
                        <>Browser: </>
                        <strong>
                          {browser.name} {browser.version}
                        </strong>
                      </span>
                    )}
                  </Stack>

                  <Pill
                    css={styles.httpStatusPill}
                    type={httpStatusColor(auditLog.status_code)}
                    text={auditLog.status_code.toString()}
                  />
                </Stack>
              </Stack>
            </Stack>
          </Stack>

          {shouldDisplayDiff ? (
            <div> {<DropdownArrow close={isDiffOpen} />}</div>
          ) : (
            <div css={styles.columnWithoutDiff}></div>
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

const styles = {
  auditLogCell: {
    padding: "0 !important",
    border: 0,
  },

  auditLogHeader: {
    padding: "16px 32px",
  },

  auditLogHeaderInfo: {
    flex: 1,
  },

  auditLogSummary: (theme) => ({
    ...(theme.typography.body1 as CSSObject),
    fontFamily: "inherit",
  }),

  auditLogTime: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
  }),

  auditLogInfo: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    fontSize: 12,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  }),

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

  deletedLabel: (theme) => ({
    ...(theme.typography.caption as CSSObject),
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
