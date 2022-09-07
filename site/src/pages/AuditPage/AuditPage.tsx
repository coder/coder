import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useSearchParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { auditMachine } from "xServices/audit/auditXService"
import { AuditPageView } from "./AuditPageView"

const AuditPage: FC = () => {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const currentPage = searchParams.get("page") ? Number(searchParams.get("page")) : 1
  const [auditState, auditSend] = useMachine(auditMachine, {
    context: {
      page: currentPage,
      limit: 25,
    },
    actions: {
      onPageChange: ({ page }) => {
        navigate({
          search: `?page=${page}`,
        })
      },
    },
  })
  const { auditLogs, count, page, limit } = auditState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Audit")}</title>
      </Helmet>
      <AuditPageView
        auditLogs={auditLogs}
        count={count}
        page={page}
        limit={limit}
        onNext={() => {
          auditSend("NEXT")
        }}
        onPrevious={() => {
          auditSend("PREVIOUS")
        }}
        onGoToPage={(page) => {
          console.log("PAGE", page)
          auditSend("GO_TO_PAGE", { page })
        }}
      />
    </>
  )
}

export default AuditPage
