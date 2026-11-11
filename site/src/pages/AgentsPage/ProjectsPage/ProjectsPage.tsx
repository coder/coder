import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjectSummaries,
	createChatProject,
	deleteChatProject,
	pinChatProject,
	unpinChatProject,
	updateChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject, ChatProjectSummary } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useSearchParamsKey } from "#/hooks/useSearchParamsKey";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { draftStorageKeys } from "../components/AgentCreateForm";
import { AgentPageHeader } from "../components/AgentPageHeader";
import {
	ChatProjectDialog,
	type ChatProjectFormValues,
} from "../components/ChatsSidebar/dialogs/ChatProjectDialog";
import { normalizeLocationSearch } from "../components/ChatsSidebar/locationSearch";
import { type ProjectsByTab, ProjectsPageView } from "./ProjectsPageView";

const tabSearchParam = "tab";

type DialogState = { readonly project?: ChatProject; readonly open: boolean };

const compareNames = (a: ChatProjectSummary, b: ChatProjectSummary) =>
	a.project.name.localeCompare(b.project.name, "en-US", {
		sensitivity: "accent",
	});

// Projects without chats have no activity and sort as the oldest.
const activityTime = (summary: ChatProjectSummary) =>
	summary.last_activity_at ? Date.parse(summary.last_activity_at) : 0;

const compareSummaries = (a: ChatProjectSummary, b: ChatProjectSummary) => {
	if (a.pinned !== b.pinned) {
		return a.pinned ? -1 : 1;
	}
	if (a.pinned) {
		return a.pin_order - b.pin_order || compareNames(a, b);
	}
	return activityTime(b) - activityTime(a) || compareNames(a, b);
};

/**
 * Each tab lists pinned projects first in pin order, then the rest by latest
 * activity, then projects without chats, with ties broken by name.
 */
export const getProjectsByTab = (
	summaries: readonly ChatProjectSummary[],
	userId: string,
): ProjectsByTab => {
	const sorted = summaries.toSorted(compareSummaries);
	return {
		yours: sorted.filter((summary) => summary.project.owner_id === userId),
		shared: sorted.filter((summary) => summary.project.owner_id !== userId),
	};
};

const ProjectsPage: React.FC = () => {
	const { user } = useAuthenticated();
	const { organizations } = useDashboard();
	const location = useLocation();
	const queryClient = useQueryClient();
	const tabState = useSearchParamsKey({ key: tabSearchParam });
	const summariesQuery = useQuery(chatProjectSummaries());
	const createMutation = useMutation(createChatProject(queryClient));
	const updateMutation = useMutation(updateChatProject(queryClient));
	// Each dialog keeps its project after closing so the exit animation does
	// not flash empty. Without a project, the project dialog creates one.
	const [projectDialog, setProjectDialog] = useState<DialogState>({
		open: false,
	});
	const [deleteDialog, setDeleteDialog] = useState<DialogState>({
		open: false,
	});
	const newProjectButtonRef = useRef<HTMLButtonElement>(null);
	// The dialogs have no trigger to return focus to, so they return it to
	// the button that opened them.
	const returnFocusRef = useRef<HTMLElement | null>(null);
	const deleteMutation = useMutation({
		...deleteChatProject(queryClient),
		onSuccess: (_data, project) => {
			// Nothing can open the deleted project's composer again.
			const draftKeys = draftStorageKeys(project.id);
			localStorage.removeItem(draftKeys.text);
			localStorage.removeItem(draftKeys.attachments);
			// The dialog can be dismissed while the delete runs and reopened for
			// another project, which must stay open.
			if (deleteDialog.project?.id === project.id) {
				setDeleteDialog({ project, open: false });
				// The project's card is gone, so focus moves to New project.
				requestAnimationFrame(() => newProjectButtonRef.current?.focus());
			}
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to delete project."));
		},
	});
	const pinOptions = pinChatProject(queryClient);
	const pinMutation = useMutation({
		...pinOptions,
		onError: (error, variables, context) => {
			pinOptions.onError?.(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to pin project."));
		},
	});
	const unpinMutation = useMutation({
		...unpinChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unpin project."));
		},
	});
	const restoreFocus = () => {
		const target = returnFocusRef.current;
		requestAnimationFrame(() => target?.focus());
	};

	// Project links keep the sidebar filters but not this page's tab.
	const linkSearchParams = new URLSearchParams(location.search);
	linkSearchParams.delete(tabSearchParam);
	const projectLinkSearch = normalizeLocationSearch(
		linkSearchParams.toString(),
	);
	const projectsByTab = summariesQuery.data
		? getProjectsByTab(summariesQuery.data, user.id)
		: undefined;
	const projectMutation = projectDialog.project
		? updateMutation
		: createMutation;

	const openProjectDialog = (
		project: ChatProject | undefined,
		returnFocusTo: HTMLElement | null,
	) => {
		returnFocusRef.current = returnFocusTo;
		createMutation.reset();
		updateMutation.reset();
		setProjectDialog({ project, open: true });
	};
	const closeProjectDialog = () => {
		setProjectDialog((current) => ({ ...current, open: false }));
		restoreFocus();
	};
	// Returning the save's promise keeps Save disabled until it settles.
	const handleProjectSubmit = async ({
		organizationId,
		...request
	}: ChatProjectFormValues) => {
		const { project } = projectDialog;
		if (project) {
			await updateMutation.mutateAsync({ project, request });
		} else if (organizationId) {
			await createMutation.mutateAsync({ organizationId, request });
		}
		closeProjectDialog();
	};

	const closeDeleteDialog = () => {
		setDeleteDialog((current) => ({ ...current, open: false }));
		restoreFocus();
	};
	return (
		<>
			<title>{pageTitle("Projects", "Agents")}</title>
			<AgentPageHeader />
			<ProjectsPageView
				tab={tabState.value === "shared" ? "shared" : "yours"}
				onTabChange={(tab) =>
					tab === "shared" ? tabState.setValue(tab) : tabState.deleteValue()
				}
				projects={projectsByTab}
				error={summariesQuery.error}
				onRetry={() => void summariesQuery.refetch()}
				projectLinkSearch={projectLinkSearch}
				newProjectButtonRef={newProjectButtonRef}
				onCreateProject={() =>
					openProjectDialog(undefined, newProjectButtonRef.current)
				}
				onPin={(project) => pinMutation.mutate({ project })}
				onUnpin={(project) => unpinMutation.mutate(project)}
				onEdit={openProjectDialog}
				onDelete={(project, returnFocusTo) => {
					returnFocusRef.current = returnFocusTo;
					setDeleteDialog({ project, open: true });
				}}
			/>
			<ChatProjectDialog
				organizations={organizations}
				initialOrganizationId={
					getDefaultOrganizationId(organizations) ?? organizations[0]?.id
				}
				project={projectDialog.project}
				open={projectDialog.open}
				onOpenChange={(open) => {
					if (!open) closeProjectDialog();
				}}
				isSubmitting={projectMutation.isPending}
				error={projectMutation.error}
				onSubmit={handleProjectSubmit}
			/>
			<DeleteDialog
				isOpen={deleteDialog.open}
				onConfirm={() =>
					deleteDialog.project && deleteMutation.mutate(deleteDialog.project)
				}
				onCancel={closeDeleteDialog}
				entity="project"
				name={deleteDialog.project?.name ?? ""}
				confirmLoading={deleteMutation.isPending}
				info="Its memories and instructions will be deleted. Chats in it, including chats started by people it is shared with, will be kept and move back to each owner's Chats list."
			/>
		</>
	);
};

export default ProjectsPage;
