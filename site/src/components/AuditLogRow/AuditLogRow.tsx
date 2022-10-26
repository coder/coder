import Collapse from "@material-ui/core/Collapse"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/typesGenerated"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { Stack } from "components/Stack/Stack"
import { useState } from "react"
import userAgentParser from "ua-parser-js"
import { combineClasses } from "util/combineClasses"
import { AuditLogAvatar } from "./AuditLogAvatar"
import { AuditLogDiff } from "./AuditLogDiff"

const readableActionMessage = (auditLog: AuditLog) => {
  return auditLog.description
    .replace("{user}", `<strong>${auditLog.user?.username.trim()}</strong>`)
    .replace("{target}", `<strong>${auditLog.resource_target.trim()}</strong>`)
}

export interface AuditLogRowProps {
  auditLog: AuditLog
  // Useful for Storybook
  defaultIsDiffOpen?: boolean
}

export const AuditLogRow: React.FC<AuditLogRowProps> = ({
  auditLog,
  defaultIsDiffOpen = false,
}) => {
  const styles = useStyles()
  const [isDiffOpen, setIsDiffOpen] = useState(defaultIsDiffOpen)
  const diffs = Object.entries(auditLog.diff)
  const shouldDisplayDiff = diffs.length > 0
  const { os, browser } = userAgentParser(auditLog.user_agent)
  const notAvailableLabel = "Not available"
  const displayBrowserInfo = browser.name
    ? `${browser.name} ${browser.version}`
    : notAvailableLabel

  const toggle = () => {
    if (shouldDisplayDiff) {
      setIsDiffOpen((v) => !v)
    }
  }

  return (
    <TableRow
      key={auditLog.id}
      data-testid={`audit-log-row-${auditLog.id}`}
      className={styles.auditLogRow}
    >
      <TableCell className={styles.auditLogCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={combineClasses({
            [styles.auditLogHeader]: true,
            [styles.clickable]: shouldDisplayDiff,
          })}
          tabIndex={0}
          onClick={toggle}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              toggle()
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
              <AuditLogAvatar auditLog={auditLog} />

              <Stack
                alignItems="baseline"
                className={styles.fullWidth}
                justifyContent="space-between"
                direction="row"
              >
                <Stack
                  className={styles.auditLogResume}
                  direction="row"
                  alignItems="baseline"
                  spacing={1}
                >
                  <span
                    dangerouslySetInnerHTML={{
                      __html: readableActionMessage(auditLog),
                    }}
                  />
                  <span className={styles.auditLogTime}>
                    {new Date(auditLog.time).toLocaleTimeString()}
                  </span>
                </Stack>
                <Stack direction="row" spacing={1}>
                  <span className={styles.auditLogInfo}>
                    IP: <strong>{auditLog.ip ?? notAvailableLabel}</strong>
                  </span>

                  <span className={styles.auditLogInfo}>
                    OS: <strong>{os.name ?? notAvailableLabel}</strong>
                  </span>

                  <span className={styles.auditLogInfo}>
                    Browser: <strong>{displayBrowserInfo}</strong>
                  </span>

                  <span className={styles.auditLogInfo}>
                    Status: <strong>{auditLog.status_code}</strong>
                  </span>
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
            <AuditLogDiff diff={auditLog.diff} />
          </Collapse>
        )}
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  auditLogCell: {
    padding: "0 !important",
    border: 0,
  },

  auditLogRow: {
    position: "relative",

    "&:focus": {
      outlineStyle: "solid",
      outlineOffset: -1,
      outlineWidth: 2,
      outlineColor: theme.palette.secondary.dark,
    },

    "&:not(:last-child) td:before": {
      position: "absolute",
      top: 20,
      left: 50,
      display: "block",
      content: "''",
      height: "100%",
      width: 2,
      background: theme.palette.divider,
    },
  },

  auditLogHeader: {
    padding: theme.spacing(2, 4),
  },

  clickable: {
    cursor: "pointer",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },

  auditLogHeaderInfo: {
    flex: 1,
  },

  auditLogResume: {
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
}))
