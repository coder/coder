import { useMachine } from "@xstate/react"
import { FC } from "react"
import { auditMachine } from "xServices/audit/auditXService"
import { AuditPageView } from "./AuditPageView"

const AuditPage: FC = () => {
  const [auditState, auditSend] = useMachine(auditMachine, {
    context: {
      page: 1,
      limit: 25,
    },
  })
  const { auditLogs, count, page, limit } = auditState.context

  return (
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
        auditSend("GO_TO_PAGE", { page })
      }}
    />
  )
}

export default AuditPage
