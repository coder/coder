import { compareAsc } from "date-fns"
import { Workspace, Template } from "api/typesGenerated"
import { TemplateScheduleFormValues } from "./formHelpers"
import { useWorkspacesData } from "pages/WorkspacesPage/data"

export const useWorkspacesToBeLocked = (
  template: Template,
  formValues: TemplateScheduleFormValues,
  fromDate: Date,
) => {
  const { data } = useWorkspacesData({
    page: 0,
    limit: 0,
    query: "template:" + template.name,
  })

  return data?.workspaces?.filter((workspace: Workspace) => {
    if (!formValues.inactivity_ttl_ms) {
      return
    }

    if (workspace.locked_at) {
      return
    }

    const proposedLocking = new Date(
      new Date(workspace.last_used_at).getTime() +
        formValues.inactivity_ttl_ms * DayInMS,
    )

    if (compareAsc(proposedLocking, fromDate) < 1) {
      return workspace
    }
  })
}

const DayInMS = 86400000

export const useWorkspacesToBeDeleted = (
  template: Template,
  formValues: TemplateScheduleFormValues,
  fromDate: Date,
) => {
  const { data } = useWorkspacesData({
    page: 0,
    limit: 0,
    query: "template:" + template.name + " locked_at:1970-01-01",
  })
  return data?.workspaces?.filter((workspace: Workspace) => {
    if (!workspace.locked_at || !formValues.locked_ttl_ms) {
      return false
    }

    const proposedLocking = new Date(
      new Date(workspace.locked_at).getTime() +
        formValues.locked_ttl_ms * DayInMS,
    )

    if (compareAsc(proposedLocking, fromDate) < 1) {
      return workspace
    }
  })
}
