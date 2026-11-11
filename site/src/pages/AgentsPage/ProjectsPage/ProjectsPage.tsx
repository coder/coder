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

const compareNames = (a: ChatProjectSummary, b: ChatProjectSummary) =>
	a.project.name
		.toLowerCase()
		.localeCompare(b.project.name.toLowerCase(), "en-US");

const activityTime = (summary: ChatProjectSummary) =>
	summary.last_activity_at ? Date.parse(summary.last_activity_at) : undefined;

const compareSummaries = (a: ChatProjectSummary, b: ChatProjectSummary) => {
	if (a.pinned !== b.pinned) {
		return a.pinned ? -1 : 1;
	}
	if (a.pinned) {
		return a.pin_order - b.pin_order || compareNames(a, b);
	}
	const aTime = activityTime(a);
	const bTime = activityTime(b);
	// Projects without chats have no activity and go last.
	if (aTime === undefined || bTime === undefined) {
		if (aTime === bTime) {
			return compareNames(a, b);
		}
		return aTime === undefined ? 1 : -1;
	}
	return bTime - aTime || compareNames(a, b);
};

/**
 * Splits the summaries into the user's own projects and others' projects.
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
	const deleteMutation = useMutation(deleteChatProject(queryClient));
	// Pinning without a pin order writes nothing optimistically, so there is
	// nothing to roll back.
	const pinMutation = useMutation({
		...pinChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to pin project."));
		},
	});
	const unpinMutation = useMutation({
		...unpinChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unpin project."));
		},
	});
	// Each dialog keeps its project after closing so the exit animation does
	// not flash empty. Without a project, the project dialog creates one.
	const [projectDialog, setProjectDialog] = useState<{
		project?: ChatProject;
		open: boolean;
	}>({ open: false });
	const [deleteDialog, setDeleteDialog] = useState<{
		project?: ChatProject;
		open: boolean;
	}>({ open: false });
	// The dialogs have no trigger to return focus to, so they return it to
	// the button that opened them.
	const returnFocusRef = useRef<HTMLElement | null>(null);
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
	const handleProjectSubmit = ({
		organizationId,
		...request
	}: ChatProjectFormValues) => {
		const { project } = projectDialog;
		if (project) {
			updateMutation.mutate(
				{ project, request },
				{ onSuccess: closeProjectDialog },
			);
		} else if (organizationId) {
			createMutation.mutate(
				{ organizationId, request },
				{ onSuccess: closeProjectDialog },
			);
		}
	};

	const closeDeleteDialog = () => {
		setDeleteDialog((current) => ({ ...current, open: false }));
		restoreFocus();
	};
	const handleDeleteProject = () => {
		const { project } = deleteDialog;
		if (!project) {
			return;
		}
		deleteMutation.mutate(project, {
			onSuccess: () => {
				// Nothing can open the deleted project's composer again.
				const draftKeys = draftStorageKeys(project.id);
				localStorage.removeItem(draftKeys.text);
				localStorage.removeItem(draftKeys.attachments);
				setDeleteDialog((current) =>
					current.project?.id === project.id
						? { ...current, open: false }
						: current,
				);
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, "Failed to delete project."));
			},
		});
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
				onCreateProject={(returnFocusTo) =>
					openProjectDialog(undefined, returnFocusTo)
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
				onConfirm={handleDeleteProject}
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
