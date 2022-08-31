import Collapse from "@material-ui/core/Collapse"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/api"
import { CodeExample } from "components/CodeExample/CodeExample"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { AuditHelpTooltip } from "components/Tooltips"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { FC, useState } from "react"
import { createDayString } from "util/createDayString"

const AuditDiff = () => {
  const styles = useStyles()

  return (
    <div className={styles.diff}>
      <div className={styles.diffOld}>
        <div className={styles.diffRow}>
          <div className={styles.diffLine}>1</div>
          <div className={styles.diffIcon}>-</div>
          <div className={styles.diffContent}>
            workspace_name: <span>alice-workspace</span>
          </div>
        </div>
      </div>
      <div className={styles.diffNew}>
        <div className={styles.diffRow}>
          <div className={styles.diffLine}>1</div>
          <div className={styles.diffIcon}>+</div>
          <div className={styles.diffContent}>
            workspace_name: <span>bruno-workspace</span>
          </div>
        </div>
      </div>
    </div>
  )
}

const AuditLogRow: React.FC<{ auditLog: AuditLog }> = ({ auditLog }) => {
  const styles = useStyles()
  const [isDiffOpen, setIsDiffOpen] = useState(false)
  const diffs = Object.entries(auditLog.diff)
  const shouldDisplayDiff = diffs.length > 1

  const toggle = () => {
    if (shouldDisplayDiff) {
      setIsDiffOpen((v) => !v)
    }
  }

  return (
    <TableRow key={auditLog.id} hover={shouldDisplayDiff}>
      <TableCell className={styles.auditLogCell}>
        <Stack
          style={{ cursor: shouldDisplayDiff ? "pointer" : undefined }}
          direction="row"
          alignItems="center"
          className={styles.auditLogRow}
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
            justifyContent="space-between"
            className={styles.auditLogRowInfo}
          >
            <Stack direction="row" alignItems="center">
              <UserAvatar username={auditLog.user?.username ?? ""} />
              <div>
                <span className={styles.auditLogResume}>
                  <strong>{auditLog.user?.username}</strong> {auditLog.action}{" "}
                  <strong>{auditLog.resource.name}</strong>
                </span>
                <span className={styles.auditLogTime}>{createDayString(auditLog.time)}</span>
              </div>
            </Stack>

            <Stack direction="column" alignItems="flex-end" spacing={1}>
              <Pill type="success" text={auditLog.status_code.toString()} />
              <Stack direction="row" alignItems="center" className={styles.auditLogExtraInfo}>
                <div>
                  <strong>IP</strong> {auditLog.ip}
                </div>
                <div>
                  <strong>Agent</strong> {auditLog.user_agent}
                </div>
              </Stack>
            </Stack>
          </Stack>

          <div className={shouldDisplayDiff ? undefined : styles.disabledDropdownIcon}>
            {isDiffOpen ? <CloseDropdown /> : <OpenDropdown />}
          </div>
        </Stack>

        {shouldDisplayDiff && (
          <Collapse in={isDiffOpen}>
            <AuditDiff />
          </Collapse>
        )}
      </TableCell>
    </TableRow>
  )
}

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
  tooltipTitle: "Copy to clipboard and try the Coder CLI",
}

export const AuditPageView: FC<{ auditLogs?: AuditLog[] }> = ({ auditLogs }) => {
  return (
    <Margins>
      <PageHeader
        actions={
          <CodeExample tooltipTitle={Language.tooltipTitle} code="coder audit [organization_ID]" />
        }
      >
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.title}</span>
            <AuditHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        <PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
      </PageHeader>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Logs</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {auditLogs ? (
              auditLogs.map((auditLog) => <AuditLogRow auditLog={auditLog} key={auditLog.id} />)
            ) : (
              <TableLoader />
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  auditLogCell: {
    padding: "0 !important",
  },

  auditLogRow: {
    padding: theme.spacing(2, 4),
  },

  auditLogRowInfo: {
    flex: 1,
  },

  auditLogResume: {
    ...theme.typography.body1,
    fontFamily: "inherit",
    display: "block",
  },

  auditLogTime: {
    ...theme.typography.body2,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  },

  auditLogExtraInfo: {
    ...theme.typography.body2,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
  },

  disabledDropdownIcon: {
    opacity: 0.5,
  },

  diff: {
    display: "flex",
    alignItems: "flex-start",
    fontSize: theme.typography.body2.fontSize,
    borderTop: `1px solid ${theme.palette.divider}`,
  },

  diffOld: {
    backgroundColor: theme.palette.error.dark,
    color: theme.palette.error.contrastText,
    flex: 1,
    paddingTop: theme.spacing(1),
    paddingBottom: theme.spacing(1),
  },

  diffRow: {
    display: "flex",
    alignItems: "baseline",
  },

  diffLine: {
    opacity: 0.5,
    padding: theme.spacing(1),
    width: theme.spacing(8),
    textAlign: "right",
  },

  diffIcon: {
    padding: theme.spacing(1),
    width: theme.spacing(4),
    textAlign: "center",
    fontSize: theme.typography.body1.fontSize,
  },

  diffContent: {},

  diffNew: {
    backgroundColor: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
    flex: 1,
    paddingTop: theme.spacing(1),
    paddingBottom: theme.spacing(1),
  },
}))
