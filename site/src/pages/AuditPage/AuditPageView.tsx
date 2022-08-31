import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/typesGenerated"
import { AuditLogRow } from "components/AuditLogRow/AuditLogRow"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { AuditHelpTooltip } from "components/Tooltips"
import { FC } from "react"

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
