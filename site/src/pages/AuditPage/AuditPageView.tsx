import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/typesGenerated"
import { AuditLogRow } from "components/AuditLogRow/AuditLogRow"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { PaginationWidget } from "components/PaginationWidget/PaginationWidget"
import { SearchBarWithFilter } from "components/SearchBarWithFilter/SearchBarWithFilter"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { Timeline } from "components/Timeline/Timeline"
import { AuditHelpTooltip } from "components/Tooltips"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import { AuditPaywall } from "./AuditPaywall"

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
}

const presetFilters = [
  {
    query: "resource_type:workspace action:create",
    name: "Created workspaces",
  },
  { query: "resource_type:template action:create", name: "Added templates" },
  { query: "resource_type:user action:delete", name: "Deleted users" },
  {
    query: "resource_type:workspace_build action:start build_reason:initiator",
    name: "Builds started by a user",
  },
  {
    query: "resource_type:api_key action:login",
    name: "User logins",
  },
]

export interface AuditPageViewProps {
  auditLogs?: AuditLog[]
  count?: number
  filter: string
  onFilter: (filter: string) => void
  paginationRef: PaginationMachineRef
  isNonInitialPage: boolean
  isAuditLogVisible: boolean
}

export const AuditPageView: FC<AuditPageViewProps> = ({
  auditLogs,
  count,
  filter,
  onFilter,
  paginationRef,
  isNonInitialPage,
  isAuditLogVisible,
}) => {
  const { t } = useTranslation("auditLog")
  const isLoading = auditLogs === undefined || count === undefined
  const isEmpty = !isLoading && auditLogs.length === 0

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.title}</span>
            <AuditHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        <PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
      </PageHeader>

      <ChooseOne>
        <Cond condition={isAuditLogVisible}>
          <SearchBarWithFilter
            docs="https://coder.com/docs/coder-oss/latest/admin/audit-logs#filtering-logs"
            filter={filter}
            onFilter={onFilter}
            presetFilters={presetFilters}
          />

          <TableContainer>
            <Table>
              <TableBody>
                <ChooseOne>
                  <Cond condition={isLoading}>
                    <TableLoader />
                  </Cond>
                  <Cond condition={isEmpty}>
                    <ChooseOne>
                      <Cond condition={isNonInitialPage}>
                        <TableRow>
                          <TableCell colSpan={999}>
                            <EmptyState message={t("table.emptyPage")} />
                          </TableCell>
                        </TableRow>
                      </Cond>
                      <Cond>
                        <TableRow>
                          <TableCell colSpan={999}>
                            <EmptyState message={t("table.noLogs")} />
                          </TableCell>
                        </TableRow>
                      </Cond>
                    </ChooseOne>
                  </Cond>
                  <Cond>
                    {auditLogs && (
                      <Timeline
                        items={auditLogs}
                        getDate={(log) => new Date(log.time)}
                        row={(log) => (
                          <AuditLogRow key={log.id} auditLog={log} />
                        )}
                      />
                    )}
                  </Cond>
                </ChooseOne>
              </TableBody>
            </Table>
          </TableContainer>

          <PaginationWidget numRecords={count} paginationRef={paginationRef} />
        </Cond>

        <Cond>
          <AuditPaywall />
        </Cond>
      </ChooseOne>
    </Margins>
  )
}
