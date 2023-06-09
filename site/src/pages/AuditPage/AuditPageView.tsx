import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableRow from "@mui/material/TableRow"
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
import { SearchBarWithFilter } from "components/SearchBarWithFilter/SearchBarWithFilter"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { Timeline } from "components/Timeline/Timeline"
import { AuditHelpTooltip } from "components/Tooltips"
import { ComponentProps, FC } from "react"
import { useTranslation } from "react-i18next"
import { AuditPaywall } from "./AuditPaywall"
import { AuditFilter } from "./AuditFilter"
import { PaginationStatus } from "components/PaginationStatus/PaginationStatus"
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase"

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
  page: number
  limit: number
  onPageChange: (page: number) => void
  isNonInitialPage: boolean
  isAuditLogVisible: boolean
  error?: Error | unknown
  filterProps:
    | ComponentProps<typeof SearchBarWithFilter>
    | ComponentProps<typeof AuditFilter>
}

export const AuditPageView: FC<AuditPageViewProps> = ({
  auditLogs,
  count,
  page,
  limit,
  onPageChange,
  isNonInitialPage,
  isAuditLogVisible,
  error,
  filterProps,
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
          {"onFilter" in filterProps ? (
            <SearchBarWithFilter
              {...filterProps}
              docs="https://coder.com/docs/coder-oss/latest/admin/audit-logs#filtering-logs"
              presetFilters={presetFilters}
              error={error}
            />
          ) : (
            <AuditFilter {...filterProps} />
          )}

          <PaginationStatus
            isLoading={Boolean(isLoading)}
            showing={auditLogs?.length}
            total={count}
            label="audit logs"
          />

          <TableContainer>
            <Table>
              <TableBody>
                <ChooseOne>
                  {/* Error condition should just show an empty table. */}
                  <Cond condition={Boolean(error)}>
                    <TableRow>
                      <TableCell colSpan={999}>
                        <EmptyState message={t("table.noLogs")} />
                      </TableCell>
                    </TableRow>
                  </Cond>
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

          {count !== undefined && (
            <PaginationWidgetBase
              count={count}
              limit={limit}
              onChange={onPageChange}
              page={page}
            />
          )}
        </Cond>

        <Cond>
          <AuditPaywall />
        </Cond>
      </ChooseOne>
    </Margins>
  )
}
