import { PlusIcon } from "lucide-react";
import type { ChatProject, ChatProjectSummary } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { buildAgentProjectPath } from "../utils/navigation";
import { ArchivedProjectCard } from "./ArchivedProjectCard";
import { ProjectCard } from "./ProjectCard";

/**
 * The Projects page tabs: active projects the user owns, active projects of
 * others, or archived projects.
 */
export type ProjectsTab = "yours" | "shared" | "archived";

/** The project summaries on each active tab, in display order. */
export type ProjectsByTab = Readonly<
	Record<Exclude<ProjectsTab, "archived">, readonly ChatProjectSummary[]>
>;

const isProjectsTab = (value: string): value is ProjectsTab =>
	value === "yours" || value === "shared" || value === "archived";

type ProjectAction = (
	project: ChatProject,
	returnFocusTo: HTMLElement | null,
) => void;

type ProjectsPageViewProps = {
	readonly tab: ProjectsTab;
	readonly onTabChange: (tab: ProjectsTab) => void;
	/** Undefined until the summaries first load. */
	readonly projects: ProjectsByTab | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	/** The query string that project links keep, such as sidebar filters. */
	readonly projectLinkSearch: string;
	readonly newProjectButtonRef: React.Ref<HTMLButtonElement>;
	readonly onCreateProject: () => void;
	readonly onPin: (project: ChatProject) => void;
	readonly onUnpin: (project: ChatProject) => void;
	/**
	 * Projects whose pin is still pending. Their cards show a spinner and leave
	 * Pin out of the menu until the request settles.
	 */
	readonly pendingPinIds: ReadonlySet<string>;
	readonly onEdit: ProjectAction;
	readonly onArchive: ProjectAction;
	readonly onDelete: ProjectAction;
	/** Shows the Archived tab, which lists archivedProjects. */
	readonly showArchivedTab: boolean;
	/** Undefined until the archived projects first load. */
	readonly archivedProjects: readonly ChatProject[] | undefined;
	readonly archivedError: unknown;
	readonly onRetryArchived: () => void;
	readonly onUnarchive: (project: ChatProject) => void;
};

/**
 * Lists the user's projects as cards, in a tab each for owned, shared, and
 * archived projects.
 */
export const ProjectsPageView: React.FC<ProjectsPageViewProps> = ({
	tab,
	onTabChange,
	projects,
	error,
	onRetry,
	projectLinkSearch,
	newProjectButtonRef,
	onCreateProject,
	onPin,
	onUnpin,
	pendingPinIds,
	onEdit,
	onArchive,
	onDelete,
	showArchivedTab,
	archivedProjects,
	archivedError,
	onRetryArchived,
	onUnarchive,
}) => {
	const projectPath = (project: ChatProject) => ({
		pathname: buildAgentProjectPath({ projectId: project.id }),
		search: projectLinkSearch,
	});
	const summaries = tab === "archived" ? undefined : projects?.[tab];
	const tabError = tab === "archived" ? archivedError : error;
	let body: React.ReactNode = null;
	if (tab === "archived") {
		if (archivedProjects) {
			body = (
				<ProjectGrid>
					{archivedProjects.map((project) => (
						<ArchivedProjectCard
							key={project.id}
							project={project}
							to={projectPath(project)}
							onUnarchive={() => onUnarchive(project)}
							onDelete={(returnFocusTo) => onDelete(project, returnFocusTo)}
						/>
					))}
				</ProjectGrid>
			);
		} else if (!archivedError) {
			body = <ProjectGridSkeleton />;
		}
	} else if (summaries && summaries.length > 0) {
		body = (
			<ProjectGrid>
				{summaries.map((summary) => (
					<ProjectCard
						key={summary.project.id}
						summary={summary}
						to={projectPath(summary.project)}
						onPin={() => onPin(summary.project)}
						onUnpin={() => onUnpin(summary.project)}
						pinPending={pendingPinIds.has(summary.project.id)}
						onEdit={(returnFocusTo) => onEdit(summary.project, returnFocusTo)}
						onArchive={(returnFocusTo) =>
							onArchive(summary.project, returnFocusTo)
						}
						onDelete={(returnFocusTo) =>
							onDelete(summary.project, returnFocusTo)
						}
					/>
				))}
			</ProjectGrid>
		);
	} else if (summaries && tab === "yours") {
		body = (
			<EmptyProjects
				title="No projects yet"
				description="Projects group related chats and give each one shared instructions, skills, and memory."
			>
				<Button variant="outline" onClick={onCreateProject}>
					<PlusIcon />
					New project
				</Button>
			</EmptyProjects>
		);
	} else if (summaries) {
		body = (
			<EmptyProjects
				title="No shared projects yet"
				description="Projects that other users share with you appear here."
			/>
		);
	} else if (!error) {
		body = <ProjectGridSkeleton />;
	}

	return (
		<div className="order-last flex min-h-0 flex-1 flex-col overflow-auto px-4 pt-6 sm:order-0 sm:px-[85.5px]">
			<div className="mx-auto flex w-full max-w-6xl flex-col gap-6 pb-8 @container">
				<header className="flex items-center justify-between gap-4">
					<h1 className="m-0 text-2xl font-semibold text-content-primary">
						Projects
					</h1>
					<Button
						ref={newProjectButtonRef}
						variant="outline"
						onClick={onCreateProject}
					>
						<PlusIcon />
						New project
					</Button>
				</header>
				<Tabs
					value={tab}
					// Each tab change adds a history entry, so focus alone must not.
					activationMode="manual"
					onValueChange={(value) =>
						onTabChange(isProjectsTab(value) ? value : "yours")
					}
				>
					<TabsList>
						<TabsTrigger value="yours">Your projects</TabsTrigger>
						<TabsTrigger value="shared">Shared with you</TabsTrigger>
						{showArchivedTab && (
							<TabsTrigger value="archived">Archived</TabsTrigger>
						)}
					</TabsList>
					{/* Only the selected tab's panel mounts, so one serves both tabs. */}
					<TabsContent value={tab} className="pt-6">
						{Boolean(tabError) && (
							<ErrorAlert
								error={tabError}
								showDebugDetail={false}
								className="mb-6"
								actions={
									<Button
										size="sm"
										variant="outline"
										onClick={tab === "archived" ? onRetryArchived : onRetry}
									>
										Retry
									</Button>
								}
							/>
						)}
						{body}
					</TabsContent>
				</Tabs>
			</div>
		</div>
	);
};

const ProjectGridSkeleton: React.FC = () => (
	<ProjectGrid role="status">
		<span className="sr-only">Loading projects</span>
		{Array.from({ length: 6 }, (_, index) => (
			<Skeleton key={index} className="h-44 rounded-lg" />
		))}
	</ProjectGrid>
);

// Columns follow the page width, which the agents sidebar narrows.
const ProjectGrid: React.FC<React.ComponentProps<"div">> = (props) => (
	<div
		className="grid grid-cols-1 gap-4 @xl:grid-cols-2 @4xl:grid-cols-3"
		{...props}
	/>
);

type EmptyProjectsProps = {
	readonly title: string;
	readonly description: string;
	readonly children?: React.ReactNode;
};

const EmptyProjects: React.FC<EmptyProjectsProps> = ({
	title,
	description,
	children,
}) => (
	<div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
		<div aria-hidden="true" className="mb-2 grid grid-cols-2 gap-1.5">
			{Array.from({ length: 4 }, (_, index) => (
				<div
					key={index}
					className="size-6 rounded-md border border-solid border-border"
				/>
			))}
		</div>
		<h2 className="m-0 text-base font-medium text-content-primary">{title}</h2>
		<p className="m-0 max-w-md text-sm text-content-secondary">{description}</p>
		{children && <div className="mt-4">{children}</div>}
	</div>
);
