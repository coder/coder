import { useQuery } from "@tanstack/react-query"
import { getWorkspaces } from "api/api"
import { compareAsc, add, endOfToday } from "date-fns"
import { WorkspaceStatus, Workspace } from "api/typesGenerated"
import { TemplateScheduleFormValues } from "./formHelpers"

const inactiveStatuses: WorkspaceStatus[] = [
  "stopped",
  "canceled",
  "failed",
  "deleted",
]

export const useWorkspacesToBeDeleted = (
  formValues: TemplateScheduleFormValues,
) => {
  const { data: workspacesData } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => getWorkspaces({}),
    enabled: formValues.inactivity_cleanup_enabled,
  })
  return workspacesData?.workspaces?.filter((workspace: Workspace) => {
    const isInactive = inactiveStatuses.includes(workspace.latest_build.status)

    const proposedDeletion = add(new Date(workspace.last_used_at), {
      days: formValues.inactivity_ttl_ms,
    })

    if (isInactive && compareAsc(proposedDeletion, endOfToday()) < 1) {
      return workspace
    }
  })
}
