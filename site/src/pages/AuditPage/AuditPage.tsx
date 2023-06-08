import { nonInitialPage } from "components/PaginationWidget/utils"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useSearchParams } from "react-router-dom"
import { pageTitle } from "utils/page"
import { AuditPageView } from "./AuditPageView"
import { useUserFilterMenu } from "components/Filter/UserFilter"
import { useFilter } from "components/Filter/filter"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { usePagination } from "hooks"
import { useQuery } from "@tanstack/react-query"
import { getAuditLogs } from "api/api"
import { useActionFilterMenu, useResourceTypeFilterMenu } from "./AuditFilter"

const AuditPage: FC = () => {
  const dashboard = useDashboard()
  const searchParamsResult = useSearchParams()
  const pagination = usePagination({ searchParamsResult })
  const filter = useFilter({
    searchParamsResult,
    onUpdate: () => {
      pagination.goToPage(1)
    },
  })
  const userMenu = useUserFilterMenu({
    value: filter.values.username,
    onChange: (option) =>
      filter.update({
        ...filter.values,
        username: option?.value,
      }),
  })
  const actionMenu = useActionFilterMenu({
    value: filter.values.action,
    onChange: (option) =>
      filter.update({
        ...filter.values,
        action: option?.value,
      }),
  })
  const resourceTypeMenu = useResourceTypeFilterMenu({
    value: filter.values["resource_type"],
    onChange: (option) =>
      filter.update({
        ...filter.values,
        resource_type: option?.value,
      }),
  })
  const { audit_log: isAuditLogVisible } = useFeatureVisibility()
  const { data, error } = useQuery({
    queryKey: ["auditLogs", filter.query, pagination.page],
    queryFn: () => {
      return getAuditLogs({
        offset: pagination.page,
        limit: 25,
        q: filter.query,
      })
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle("Audit")}</title>
      </Helmet>
      <AuditPageView
        auditLogs={data?.audit_logs}
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
        isNonInitialPage={nonInitialPage(searchParamsResult[0])}
        isAuditLogVisible={isAuditLogVisible}
        error={error}
        filterProps={
          dashboard.experiments.includes("workspace_filter")
            ? {
                filter,
                menus: {
                  user: userMenu,
                  action: actionMenu,
                  resourceType: resourceTypeMenu,
                },
              }
            : {
                filter: filter.query,
                onFilter: filter.update,
              }
        }
      />
    </>
  )
}

export default AuditPage
