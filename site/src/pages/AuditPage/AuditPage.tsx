import { useMachine } from "@xstate/react"
import {
  getPaginationContext,
  nonInitialPage,
} from "components/PaginationWidget/utils"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useSearchParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { auditMachine } from "xServices/audit/auditXService"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import { AuditPageView } from "./AuditPageView"
import { RequirePermission } from "components/RequirePermission/RequirePermission"
import { useReadPagePermissions } from "hooks/useReadPagePermissions"
import { Loader } from "components/Loader/Loader"

const AuditPage: FC = () => {
  // we call the below hook to make sure the user has access to view the page
  const { data: permissions, isLoading: isLoadingPermissions } =
    useReadPagePermissions("audit_log")

  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? ""
  const [auditState, auditSend] = useMachine(auditMachine, {
    context: {
      filter,
      paginationContext: getPaginationContext(searchParams),
    },
    actions: {
      updateURL: (context, event) =>
        setSearchParams({ page: event.page, filter: context.filter }),
    },
  })

  const { auditLogs, count, apiError } = auditState.context
  const paginationRef = auditState.context.paginationRef as PaginationMachineRef
  const { audit_log: isAuditLogEnabled } = useFeatureVisibility()

  if (!permissions || isLoadingPermissions) {
    return <Loader />
  }

  return (
    <RequirePermission
      isFeatureVisible={isAuditLogEnabled && permissions.readPagePermissions}
    >
      <>
        <Helmet>
          <title>{pageTitle("Audit")}</title>
        </Helmet>
        <AuditPageView
          filter={filter}
          auditLogs={auditLogs}
          count={count}
          onFilter={(filter) => {
            auditSend("FILTER", { filter })
          }}
          paginationRef={paginationRef}
          isNonInitialPage={nonInitialPage(searchParams)}
          isAuditLogVisible={isAuditLogEnabled}
          error={apiError}
        />
      </>
    </RequirePermission>
  )
}

export default AuditPage
