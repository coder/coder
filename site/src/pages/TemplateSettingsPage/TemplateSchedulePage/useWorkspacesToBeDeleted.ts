import type { Template, Workspace } from "api/typesGenerated";
import { compareAsc } from "date-fns";
import { calcOffset } from "hooks/usePagination";
import { useWorkspacesData } from "pages/WorkspacesPage/data";
import type { TemplateScheduleFormValues } from "./formHelpers";

export const useWorkspacesToGoDormant = (
	template: Template,
	formValues: TemplateScheduleFormValues,
	fromDate: Date,
) => {
	const { data } = useWorkspacesData({
		offset: calcOffset(0, 0),
		limit: 0,
		q: `template:${template.name}`,
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
				formValues.time_til_dormant_ms,
		);

		if (compareAsc(proposedLocking, fromDate) < 1) {
			return workspace;
		}
	});
};

export const useWorkspacesToBeDeleted = (
	template: Template,
	formValues: TemplateScheduleFormValues,
	fromDate: Date,
) => {
	const { data } = useWorkspacesData({
		offset: calcOffset(0, 0),
		limit: 0,
		q: `template:${template.name} dormant:true`,
	});
	return data?.workspaces?.filter((workspace: Workspace) => {
		if (!workspace.dormant_at || !formValues.time_til_dormant_autodelete_ms) {
			return false;
		}

		const proposedLocking = new Date(
			new Date(workspace.dormant_at).getTime() +
				formValues.time_til_dormant_autodelete_ms,
		);

		if (compareAsc(proposedLocking, fromDate) < 1) {
			return workspace;
		}
	});
};
