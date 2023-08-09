import { useQuery } from "@tanstack/react-query"
import { getWorkspaces } from "api/api"
import { compareAsc } from "date-fns"
import { Workspace, Template } from "api/typesGenerated"
import { TemplateScheduleFormValues } from "./formHelpers"

export const useWorkspacesToBeLocked = (
  template: Template,
  formValues: TemplateScheduleFormValues,
) => {
  const { data: workspacesData } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () =>
      getWorkspaces({
        q: "template:" + template.name,
      }),
    enabled: formValues.inactivity_cleanup_enabled,
  })

  return workspacesData?.workspaces?.filter((workspace: Workspace) => {
    if (!formValues.inactivity_ttl_ms) {
      return
    }

    if (workspace.locked_at) {
      return
    }

    const proposedLocking = new Date(
      new Date(workspace.last_used_at).getTime() +
        formValues.inactivity_ttl_ms * 86400000,
    )

    if (compareAsc(proposedLocking, new Date()) < 1) {
      return workspace
    }
  })
}

export const useWorkspacesToBeDeleted = (
  template: Template,
  formValues: TemplateScheduleFormValues,
) => {
  const { data: workspacesData } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () =>
      getWorkspaces({
        q: "template:" + template.name,
      }),
    enabled: formValues.locked_cleanup_enabled,
  })
  return workspacesData?.workspaces?.filter((workspace: Workspace) => {
    if (!workspace.locked_at || !formValues.locked_ttl_ms) {
      return false
    }

    const proposedLocking = new Date(
      new Date(workspace.locked_at).getTime() +
        formValues.locked_ttl_ms * 86400000,
    )

    if (compareAsc(proposedLocking, new Date()) < 1) {
      return workspace
    }
  })
}
