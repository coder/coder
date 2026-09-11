import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { NavLink, useLocation } from "react-router";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { getSectionToggleTestId } from "./ChatSectionHeader";

const PROJECTS_SECTION_KEY = "Projects";

type ProjectsSectionProps = {
	readonly projects: readonly ChatProject[];
	readonly expanded: boolean;
	readonly onToggle: () => void;
	readonly onCreate: () => void;
	readonly onEdit: (project: ChatProject) => void;
	readonly onDelete: (project: ChatProject) => void;
};

export const ProjectsSection: FC<ProjectsSectionProps> = ({
	projects,
	expanded,
	onToggle,
	onCreate,
	onEdit,
	onDelete,
}) => {
	const location = useLocation();

	return (
		<div className="not-first:mt-3">
			<div className="group/header mb-1 ml-2.5 mr-2 flex h-7 items-center text-xs font-medium text-content-secondary">
				<button
					type="button"
					className="flex h-7 min-w-0 flex-1 cursor-pointer appearance-none items-center rounded-md border-0 bg-transparent p-0 text-left font-sans text-xs font-medium text-current focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link [@media(hover:hover)]:group-hover/header:text-content-primary"
					aria-expanded={expanded}
					aria-label={`${expanded ? "Collapse" : "Expand"} Projects section`}
					data-testid={getSectionToggleTestId(PROJECTS_SECTION_KEY)}
					onClick={onToggle}
				>
					<span className="min-w-0 flex-1 truncate">
						Projects ({projects.length})
					</span>
				</button>
				<Button
					variant="subtle"
					size="icon"
					className="size-7 min-w-0"
					aria-label="New project"
					onClick={onCreate}
				>
					<PlusIcon className="size-3.5" />
				</Button>
			</div>
			{expanded && (
				<div className="flex flex-col gap-0.5">
					{projects.map((project) => (
						<ContextMenu key={project.id}>
							<ContextMenuTrigger asChild>
								<div className="group relative flex items-center gap-1 rounded-md px-2 py-1 text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary has-[[aria-current=page]]:border-l has-[[aria-current=page]]:border-content-primary has-[[aria-current=page]]:pl-[7px]">
									<NavLink
										to={{
											pathname: buildAgentProjectPath(project.id),
											search: location.search,
										}}
										className="min-w-0 flex-1 truncate text-[13px] text-content-primary no-underline"
									>
										{project.name}
									</NavLink>
									<span className="rounded bg-surface-secondary px-1.5 py-0.5 text-2xs tabular-nums">
										{project.chat_count}
									</span>
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button
												variant="subtle"
												size="icon"
												className="size-6 min-w-0 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
												aria-label={`Open actions for ${project.name}`}
											>
												<EllipsisVerticalIcon className="size-3.5" />
											</Button>
										</DropdownMenuTrigger>
										<DropdownMenuContent align="end">
											<DropdownMenuItem onSelect={() => onEdit(project)}>
												Edit project
											</DropdownMenuItem>
											<DropdownMenuItem
												className="text-content-destructive focus:text-content-destructive"
												onSelect={() => onDelete(project)}
											>
												Delete project
											</DropdownMenuItem>
										</DropdownMenuContent>
									</DropdownMenu>
								</div>
							</ContextMenuTrigger>
							<ContextMenuContent>
								<ContextMenuItem onSelect={() => onEdit(project)}>
									Edit project
								</ContextMenuItem>
								<ContextMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onSelect={() => onDelete(project)}
								>
									Delete project
								</ContextMenuItem>
							</ContextMenuContent>
						</ContextMenu>
					))}
				</div>
			)}
		</div>
	);
};
