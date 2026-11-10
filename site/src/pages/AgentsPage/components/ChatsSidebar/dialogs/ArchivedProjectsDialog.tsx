import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	archivedChatProjects,
	unarchiveChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
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
import { toast } from "#/components/Toaster/toast";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatProjectIcon } from "../../ChatProjectIcon";

type ArchivedProjectsDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** Kept on project links, since the sidebar filters live in it. */
	readonly locationSearch: string;
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/** Lists archived projects with links to them and Unarchive buttons. */
export const ArchivedProjectsDialog: React.FC<ArchivedProjectsDialogProps> = ({
	open,
	onOpenChange,
	locationSearch,
	onCloseAutoFocus,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
			<DialogHeader>
				<DialogTitle>Archived projects</DialogTitle>
				<DialogDescription>
					Unarchive a project to restore it and its chats.
				</DialogDescription>
			</DialogHeader>
			<ArchivedProjectsList
				locationSearch={locationSearch}
				onOpenProject={() => onOpenChange(false)}
			/>
		</DialogContent>
	</Dialog>
);

type ArchivedProjectsListProps = {
	readonly locationSearch: string;
	readonly onOpenProject: () => void;
};

const ArchivedProjectsList: React.FC<ArchivedProjectsListProps> = ({
	locationSearch,
	onOpenProject,
}) => {
	const queryClient = useQueryClient();
	const projectsQuery = useQuery(archivedChatProjects());
	const unarchiveMutation = useMutation({
		...unarchiveChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unarchive project."));
		},
	});
	const unarchivingProjectId = unarchiveMutation.isPending
		? unarchiveMutation.variables.id
		: undefined;

	if (projectsQuery.error && !projectsQuery.data) {
		return (
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
		);
	}
	if (!projectsQuery.data) {
		return (
			<div role="status" className="flex flex-col gap-3">
				<span className="sr-only">Loading archived projects</span>
				{[0, 1, 2].map((index) => (
					<div key={index} className="flex items-center gap-3">
						<Skeleton className="size-4 shrink-0" />
						<Skeleton variant="text" className="w-1/3" />
					</div>
				))}
			</div>
		);
	}
	if (projectsQuery.data.length === 0) {
		return (
			<p className="m-0 rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-content-secondary">
				No archived projects.
			</p>
		);
	}
	return (
		<ul className="m-0 flex list-none flex-col gap-1 p-0">
			{projectsQuery.data.map((project) => (
				<ArchivedProjectRow
					key={project.id}
					project={project}
					locationSearch={locationSearch}
					isUnarchiving={unarchivingProjectId === project.id}
					onOpen={onOpenProject}
					onUnarchive={() => unarchiveMutation.mutate(project)}
				/>
			))}
		</ul>
	);
};

type ArchivedProjectRowProps = {
	readonly project: ChatProject;
	readonly locationSearch: string;
	readonly isUnarchiving: boolean;
	readonly onOpen: () => void;
	readonly onUnarchive: () => void;
};

const ArchivedProjectRow: React.FC<ArchivedProjectRowProps> = ({
	project,
	locationSearch,
	isUnarchiving,
	onOpen,
	onUnarchive,
}) => (
	<li className="flex min-w-0 items-center gap-3 py-1">
		<ChatProjectIcon
			project={project}
			className="size-4 text-content-secondary"
		/>
		<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
			{project.name}
		</span>
		<Button asChild size="sm" variant="outline">
			<RouterLink
				to={{
					pathname: buildAgentProjectPath({ projectId: project.id }),
					search: locationSearch,
				}}
				aria-label={`Open ${project.name}`}
				onClick={onOpen}
			>
				Open
			</RouterLink>
		</Button>
		{project.permissions.archive && (
			<Button
				size="sm"
				variant="outline"
				aria-label={`Unarchive ${project.name}`}
				disabled={isUnarchiving}
				onClick={onUnarchive}
			>
				<Spinner loading={isUnarchiving} />
				Unarchive
			</Button>
		)}
	</li>
);
