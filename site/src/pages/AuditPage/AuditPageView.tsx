import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/api"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { AuditHelpTooltip } from "components/Tooltips"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { FC } from "react"
import { createDayString } from "util/createDayString"

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
  tooltipTitle: "Copy to clipboard and try the Coder CLI",
}

export const AuditPageView: FC<{ auditLogs?: AuditLog[] }> = ({ auditLogs }) => {
  const styles = useStyles()

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
              auditLogs.map((log) => (
                <TableRow key={log.id}>
                  <TableCell>
                    <Stack direction="row" alignItems="center" justifyContent="space-between">
                      <Stack direction="row" alignItems="center">
                        <UserAvatar username={log.user?.username ?? ""} />
                        <div>
                          <span className={styles.auditLogResume}>
                            <strong>{log.user?.username}</strong> {log.action}{" "}
                            <strong>{log.resource.name}</strong>
                          </span>
                          <span className={styles.auditLogTime}>{createDayString(log.time)}</span>
                        </div>
                      </Stack>

                      <Stack direction="column" alignItems="flex-end" spacing={1}>
                        <Pill type="success" text={log.status_code.toString()} />
                        <Stack
                          direction="row"
                          alignItems="center"
                          className={styles.auditLogExtraInfo}
                        >
                          <div>
                            <strong>IP</strong> {log.ip}
                          </div>
                          <div>
                            <strong>Agent</strong> {log.user_agent}
                          </div>
                        </Stack>
                      </Stack>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))
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
}))
