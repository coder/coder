import { useEffect, useRef } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { archivedChatProjects } from "#/api/queries/chatProjects";
import type { ChatProject, Organization } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnarchiveChatProject } from "../../../hooks/useUnarchiveChatProject";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatProjectIcon } from "../../ChatProjectIcon";
import {
	getOrganizationLabels,
	getProjectLabel,
} from "../chats/projectGrouping";

type ArchivedProjectsDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** Labels same-named projects with their organization. */
	readonly organizations: readonly Organization[];
	/** Kept on project links, since the sidebar filters live in it. */
	readonly locationSearch: string;
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/** Lists archived projects with links to them and Unarchive buttons. */
export const ArchivedProjectsDialog: React.FC<ArchivedProjectsDialogProps> = ({
	open,
	onOpenChange,
	organizations,
	locationSearch,
	onCloseAutoFocus,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
			<DialogHeader>
				<DialogTitle>Archived projects</DialogTitle>
				<DialogDescription>
					Archived projects you own or that are shared with you. Unarchiving a
					project restores its chats.
				</DialogDescription>
			</DialogHeader>
			<ArchivedProjectsList
				organizations={organizations}
				locationSearch={locationSearch}
				onOpenProject={() => onOpenChange(false)}
			/>
		</DialogContent>
	</Dialog>
);

type ArchivedProjectsListProps = {
	readonly organizations: readonly Organization[];
	readonly locationSearch: string;
	readonly onOpenProject: () => void;
};

const ArchivedProjectsList: React.FC<ArchivedProjectsListProps> = ({
	organizations,
	locationSearch,
	onOpenProject,
}) => {
	const projectsQuery = useQuery(archivedChatProjects());
	const projects = projectsQuery.data;
	const rowActionsRef = useRef(new Map<string, HTMLElement>());
	// Row indexes of pending unarchives, by project ID.
	const pendingUnarchivesRef = useRef(new Map<string, number>());
	const emptyStateRef = useRef<HTMLParagraphElement>(null);

	// An unarchived row unmounts with its focused button, which leaves focus on
	// the body or the dialog. Hand it to the first enabled action from the row
	// that took its place onward, else before it, else the empty state.
	useEffect(() => {
		if (!projects) {
			return;
		}
		let removedIndex: number | undefined;
		for (const [projectId, index] of pendingUnarchivesRef.current) {
			if (!projects.some((p) => p.id === projectId)) {
				pendingUnarchivesRef.current.delete(projectId);
				removedIndex = removedIndex ?? index;
			}
		}
		if (removedIndex === undefined) {
			return;
		}
		const active = document.activeElement;
		if (
			active &&
			active !== document.body &&
			active.getAttribute("role") !== "dialog"
		) {
			return;
		}
		const start = Math.min(removedIndex, projects.length);
		const candidates = [
			...projects.slice(start),
			...projects.slice(0, start).reverse(),
		].map((p) => rowActionsRef.current.get(p.id));
		const target =
			projects.length === 0
				? emptyStateRef.current
				: candidates.find(
						(action) =>
							action &&
							!(action instanceof HTMLButtonElement && action.disabled),
					);
		target?.focus();
	}, [projects]);

	const errorAlert = projectsQuery.error ? (
		<ErrorAlert
			error={projectsQuery.error}
			actions={
				<Button
					size="sm"
					variant="outline"
					onClick={() => void projectsQuery.refetch()}
				>
					Retry
				</Button>
			}
		/>
	) : null;
	if (!projects) {
		return (
			errorAlert ?? (
				<div role="status" className="flex flex-col gap-3">
					<span className="sr-only">Loading archived projects</span>
					{[0, 1, 2].map((index) => (
						<div key={index} className="flex items-center gap-3">
							<Skeleton className="size-4 shrink-0" />
							<Skeleton variant="text" className="w-1/3" />
						</div>
					))}
				</div>
			)
		);
	}
	const organizationLabels = getOrganizationLabels(projects, organizations);
	return (
		<>
			{errorAlert}
			{projects.length === 0 ? (
				<p
					ref={emptyStateRef}
					tabIndex={-1}
					className="m-0 rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-content-secondary"
				>
					No archived projects.
				</p>
			) : (
				<ul className="m-0 flex min-w-0 list-none flex-col gap-1 p-0">
					{projects.map((project, index) => (
						<ArchivedProjectRow
							key={project.id}
							project={project}
							organizationLabel={organizationLabels.get(project.id)}
							locationSearch={locationSearch}
							onOpen={onOpenProject}
							onUnarchive={() => {
								pendingUnarchivesRef.current.set(project.id, index);
							}}
							onUnarchiveError={() => {
								pendingUnarchivesRef.current.delete(project.id);
							}}
							actionRef={(element) => {
								if (!element) {
									return;
								}
								rowActionsRef.current.set(project.id, element);
								return () => {
									rowActionsRef.current.delete(project.id);
								};
							}}
						/>
					))}
				</ul>
			)}
		</>
	);
};

type ArchivedProjectRowProps = {
	readonly project: ChatProject;
	readonly organizationLabel: string | undefined;
	readonly locationSearch: string;
	readonly onOpen: () => void;
	readonly onUnarchive: () => void;
	readonly onUnarchiveError: () => void;
	/** Receives the row's last action, which takes focus from a removed row. */
	readonly actionRef: React.RefCallback<HTMLElement>;
};

const ArchivedProjectRow: React.FC<ArchivedProjectRowProps> = ({
	project,
	organizationLabel,
	locationSearch,
	onOpen,
	onUnarchive,
	onUnarchiveError,
	actionRef,
}) => {
	const { unarchive, isPending } = useUnarchiveChatProject(
		project,
		onUnarchiveError,
	);
	const canUnarchive = project.permissions.archive;
	const projectLabel = getProjectLabel(project, organizationLabel);
	return (
		<li className="flex items-center gap-3 py-1">
			<ChatProjectIcon
				project={project}
				className="size-4 text-content-secondary"
			/>
			<span className="flex min-w-0 flex-1 items-baseline gap-2 text-sm">
				<span className="truncate text-content-primary">{project.name}</span>
				{organizationLabel && (
					<span className="max-w-1/2 shrink-0 truncate text-content-secondary">
						{organizationLabel}
					</span>
				)}
			</span>
			<Button asChild size="sm" variant="outline">
				<RouterLink
					to={{
						pathname: buildAgentProjectPath({ projectId: project.id }),
						search: locationSearch,
					}}
					ref={canUnarchive ? undefined : actionRef}
					aria-label={`Open ${projectLabel}`}
					onClick={onOpen}
				>
					Open
				</RouterLink>
			</Button>
			{canUnarchive && (
				<Button
					ref={actionRef}
					size="sm"
					variant="outline"
					aria-label={`Unarchive ${projectLabel}`}
					disabled={isPending}
					onClick={() => {
						onUnarchive();
						unarchive();
					}}
				>
					<Spinner loading={isPending} />
					Unarchive
				</Button>
			)}
		</li>
	);
};
