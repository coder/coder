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
import { getOrganizationLabels } from "../chats/projectGrouping";

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

type UnarchivedRow = { readonly projectId: string; readonly index: number };

const ArchivedProjectsList: React.FC<ArchivedProjectsListProps> = ({
	organizations,
	locationSearch,
	onOpenProject,
}) => {
	const projectsQuery = useQuery(archivedChatProjects());
	const projects = projectsQuery.data;
	const rowActionsRef = useRef(new Map<string, HTMLElement>());
	const unarchivedRowRef = useRef<UnarchivedRow | null>(null);
	const emptyStateRef = useRef<HTMLParagraphElement>(null);

	// An unarchived row unmounts with its focused button, which leaves focus on
	// the body or the dialog. Hand it to the row that took its place, else the
	// previous row, else the empty state.
	useEffect(() => {
		const row = unarchivedRowRef.current;
		if (!row || !projects || projects.some((p) => p.id === row.projectId)) {
			return;
		}
		unarchivedRowRef.current = null;
		const active = document.activeElement;
		if (
			active &&
			active !== document.body &&
			active.getAttribute("role") !== "dialog"
		) {
			return;
		}
		const next = projects[Math.min(row.index, projects.length - 1)];
		const target = next
			? rowActionsRef.current.get(next.id)
			: emptyStateRef.current;
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
					className="m-0 rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-content-secondary outline-hidden"
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
								unarchivedRowRef.current = { projectId: project.id, index };
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
	/** Receives the row's last action, which takes focus from a removed row. */
	readonly actionRef: React.RefCallback<HTMLElement>;
};

const ArchivedProjectRow: React.FC<ArchivedProjectRowProps> = ({
	project,
	organizationLabel,
	locationSearch,
	onOpen,
	onUnarchive,
	actionRef,
}) => {
	const { unarchive, isPending } = useUnarchiveChatProject(project);
	const canUnarchive = project.permissions.archive;
	const projectLabel = organizationLabel
		? `${project.name} (${organizationLabel})`
		: project.name;
	return (
		<li className="flex items-center gap-3 py-1">
			<ChatProjectIcon
				project={project}
				className="size-4 text-content-secondary"
			/>
			<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
				{projectLabel}
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
