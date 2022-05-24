import { useMachine } from "@xstate/react"
import React, { useEffect } from "react"
import { useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import {
  WorkspaceScheduleForm,
  WorkspaceScheduleFormValues,
} from "../../components/WorkspaceStats/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"

// TODO(Grey): Test before opening PR from draft
export const formValuesToAutoStartRequest = (
  values: WorkspaceScheduleFormValues,
): TypesGen.UpdateWorkspaceAutostartRequest => {
  if (!values.startTime) {
    return {
      schedule: "",
    }
  }

  // TODO(Grey): Fill in
  return {
    schedule: "9 30 * * 1-5",
  }
}

export const formValuesToTTLRequest = (values: WorkspaceScheduleFormValues): TypesGen.UpdateWorkspaceTTLRequest => {
  if (!values.ttl) {
    return {
      ttl: 0, // TODO(Grey): Verify with Cian whether 0 or null is better to send
    }
  }

  // TODO(Grey): Fill in
  return {
    ttl: 0,
  }
}

// TODO(Grey): React testing library for this
export const WorkspaceSchedulePage: React.FC = () => {
  const navigate = useNavigate()
  const { workspace: workspaceQueryParam } = useParams()
  const workspaceId = firstOrItem(workspaceQueryParam, null)

  // TODO(Grey): Consume the formSubmissionErrors in WorkspaceScheduleForm
  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule)
  const { getWorkspaceError, workspace } = scheduleState.context

  // Get workspace on mount and whenever workspaceId changes.
  // scheduleSend should not change.
  useEffect(() => {
    workspaceId && scheduleSend({ type: "GET_WORKSPACE", workspaceId })
  }, [workspaceId, scheduleSend])

  if (!workspaceId) {
    navigate("/workspaces")
    return null
  } else if (scheduleState.matches("error")) {
    return <ErrorSummary error={getWorkspaceError} retry={() => scheduleSend({ type: "GET_WORKSPACE", workspaceId })} />
  } else if (!workspace) {
    return <FullScreenLoader />
  } else {
    return (
      <WorkspaceScheduleForm
        isLoading={scheduleState.tags.has("loading")}
        onCancel={() => {
          navigate(`/workspaces/${workspaceId}`)
        }}
        onSubmit={(values) => {
          scheduleSend({
            type: "SUBMIT_SCHEDULE",
            autoStart: formValuesToAutoStartRequest(values),
            ttl: formValuesToTTLRequest(values),
          })
          // TODO(Grey): navigation logic
        }}
      />
    )
  }
}
