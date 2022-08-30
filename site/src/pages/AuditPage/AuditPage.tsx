import { useMachine } from "@xstate/react"
import { FC } from "react"
import { auditMachine } from "xServices/audit/auditXService"
import { AuditPageView } from "./AuditPageView"

const AuditPage: FC = () => {
  const [auditState] = useMachine(auditMachine)
  const { auditLogs } = auditState.context

  return <AuditPageView auditLogs={auditLogs} />
}

export default AuditPage
