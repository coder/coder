import { compareAsc } from "date-fns";
import { Workspace, Template } from "api/typesGenerated";
import { TemplateScheduleFormValues } from "./formHelpers";
import { useWorkspacesData } from "pages/WorkspacesPage/data";

export const useWorkspacesToGoDormant = (
  template: Template,
  formValues: TemplateScheduleFormValues,
  fromDate: Date,
) => {
  const { data } = useWorkspacesData({
    page: 0,
    limit: 0,
    query: "template:" + template.name,
  });

  return data?.workspaces?.filter((workspace: Workspace) => {
    if (!formValues.time_til_dormant_ms) {
      return;
    }

    if (workspace.dormant_at) {
      return;
    }

    const proposedLocking = new Date(
      new Date(workspace.last_used_at).getTime() +
        formValues.time_til_dormant_ms * DayInMS,
    );

    if (compareAsc(proposedLocking, fromDate) < 1) {
      return workspace;
    }
  });
};

const DayInMS = 86400000;

export const useWorkspacesToBeDeleted = (
  template: Template,
  formValues: TemplateScheduleFormValues,
  fromDate: Date,
) => {
  const { data } = useWorkspacesData({
    page: 0,
    limit: 0,
    query: "template:" + template.name + " dormant_at:1970-01-01",
  });
  return data?.workspaces?.filter((workspace: Workspace) => {
    if (!workspace.dormant_at || !formValues.time_til_dormant_autodelete_ms) {
      return false;
    }

    const proposedLocking = new Date(
      new Date(workspace.dormant_at).getTime() +
        formValues.time_til_dormant_autodelete_ms * DayInMS,
    );

    if (compareAsc(proposedLocking, fromDate) < 1) {
      return workspace;
    }
  });
};
