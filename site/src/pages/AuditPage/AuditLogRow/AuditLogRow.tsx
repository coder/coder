import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import TableCell from "@mui/material/TableCell";
import Tooltip from "@mui/material/Tooltip";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import userAgentParser from "ua-parser-js";
import type { AuditLog } from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import type { ThemeRole } from "theme/roles";
import { AuditLogDescription } from "./AuditLogDescription/AuditLogDescription";
import { AuditLogDiff } from "./AuditLogDiff/AuditLogDiff";
import { determineGroupDiff } from "./AuditLogDiff/auditUtils";

const httpStatusColor = (httpStatus: number): ThemeRole => {
  // Treat server errors (500) as errors
  if (httpStatus >= 500) {
    return "error";
  }

  // Treat client errors (400) as warnings
  if (httpStatus >= 400) {
    return "warning";
  }

  // OK (200) and redirects (300) are successful
  return "success";
};

export interface AuditLogRowProps {
  auditLog: AuditLog;
  // Useful for Storybook
  defaultIsDiffOpen?: boolean;
  showOrgDetails: boolean;
}

export const AuditLogRow: FC<AuditLogRowProps> = ({
  auditLog,
  defaultIsDiffOpen = false,
  showOrgDetails,
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
                  {/* With multi-org, there is not enough space so show
                      everything in a tooltip. */}
                  {showOrgDetails ? (
                    <Tooltip
                      title={
                        <div css={styles.auditLogInfoTooltip}>
                          {auditLog.ip && (
                            <div>
                              <h4 css={styles.auditLogInfoHeader}>IP:</h4>
                              <div>{auditLog.ip}</div>
                            </div>
                          )}
                          {os.name && (
                            <div>
                              <h4 css={styles.auditLogInfoHeader}>OS:</h4>
                              <div>{os.name}</div>
                            </div>
                          )}
                          {browser.name && (
                            <div>
                              <h4 css={styles.auditLogInfoHeader}>Browser:</h4>
                              <div>
                                {browser.name} {browser.version}
                              </div>
                            </div>
                          )}
                          {auditLog.organization && (
                            <div>
                              <h4 css={styles.auditLogInfoHeader}>
                                Organization:
                              </h4>
                              <Link
                                component={RouterLink}
                                to={`/organizations/${auditLog.organization.name}`}
                              >
                                {auditLog.organization.display_name ||
                                  auditLog.organization.name}
                              </Link>
                            </div>
                          )}
                        </div>
                      }
                    >
                      <InfoOutlined
                        css={(theme) => ({
                          fontSize: 20,
                          color: theme.palette.info.light,
                        })}
                      />
                    </Tooltip>
                  ) : (
                    <Stack direction="row" spacing={1} alignItems="baseline">
                      {auditLog.ip && (
                        <span css={styles.auditLogInfo}>
                          <span>IP: </span>
                          <strong>{auditLog.ip}</strong>
                        </span>
                      )}
                      {os.name && (
                        <span css={styles.auditLogInfo}>
                          <span>OS: </span>
                          <strong>{os.name}</strong>
                        </span>
                      )}
                      {browser.name && (
                        <span css={styles.auditLogInfo}>
                          <span>Browser: </span>
                          <strong>
                            {browser.name} {browser.version}
                          </strong>
                        </span>
                      )}
                    </Stack>
                  )}

                  <Pill
                    css={styles.httpStatusPill}
                    type={httpStatusColor(auditLog.status_code)}
                  >
                    {auditLog.status_code.toString()}
                  </Pill>
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

  auditLogInfoHeader: (theme) => ({
    margin: 0,
    color: theme.palette.text.primary,
    fontSize: 14,
    lineHeight: "150%",
    fontWeight: 600,
  }),

  auditLogInfoTooltip: {
    display: "flex",
    flexDirection: "column",
    gap: 8,
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

  deletedLabel: (theme) => ({
    ...(theme.typography.caption as CSSObject),
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
