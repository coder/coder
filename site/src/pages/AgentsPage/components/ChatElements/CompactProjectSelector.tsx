import { cn } from "cn";
import {
	CheckIcon,
	FolderOpenIcon,
	MessageCircleIcon,
	PlusIcon,
} from "lucide-react";
import { useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type { ChatProject } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { ChatProjectIcon } from "../ChatProjectIcon";

const noProjectLabel = "No project";

// Items use IDs as their cmdk values because project names are not unique, so
// search matches only the names passed as keywords.
const filterByKeywords = (
	_value: string,
	search: string,
	keywords?: string[],
) => {
	const query = search.trim().toLowerCase();
	if (!query) {
		return 1;
	}
	return keywords?.some((keyword) => keyword.toLowerCase().includes(query))
		? 1
		: 0;
};

type CompactProjectSelectorProps = {
	/** The selected project, or null for no project. */
	readonly value: ChatProject | null;
	/** The projects the user can pick, already scoped to the organization. */
	readonly options: readonly ChatProject[];
	readonly onChange: (project: ChatProject | null) => void;
	readonly onCreateProject: () => void;
	readonly isLoading?: boolean;
	/** Shown in place of the project list. */
	readonly error?: unknown;
	readonly onRetry?: () => void;
	readonly disabled?: boolean;
};

/** Picks the project a new chat joins, if any. */
export const CompactProjectSelector: React.FC<CompactProjectSelectorProps> = ({
	value,
	options,
	onChange,
	onCreateProject,
	isLoading = false,
	error,
	onRetry,
	disabled = false,
}) => {
	const [open, setOpen] = useState(false);
	const label = value?.name ?? noProjectLabel;
	const select = (project: ChatProject | null) => {
		onChange(project);
		setOpen(false);
	};

	return (
		<Popover open={open} onOpenChange={disabled ? undefined : setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					disabled={disabled}
					aria-label={`Project: ${label}`}
					className={cn(
						"group flex h-6 w-auto min-w-0 cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none whitespace-nowrap transition-colors",
						"rounded-sm hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
						"disabled:cursor-not-allowed disabled:opacity-50",
					)}
				>
					{value ? (
						<ChatProjectIcon project={value} className="size-3.5" />
					) : (
						<FolderOpenIcon aria-hidden="true" className="size-3.5 shrink-0" />
					)}
					<span className="truncate">{label}</span>
					<ChevronDownIcon
						open={open}
						className="size-icon-sm shrink-0 text-content-secondary transition-colors hover:text-content-primary group-hover:text-content-primary"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				align="start"
				className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-72 max-w-[calc(100vw-2rem)] overflow-hidden p-0"
				onOpenAutoFocus={(event) => {
					// Focusing the search input on touch devices opens the software
					// keyboard, which hides the list.
					if (matchMedia("(pointer: coarse)").matches) {
						event.preventDefault();
					}
				}}
			>
				<Command
					size="sm"
					loop
					filter={filterByKeywords}
					label="Search projects"
				>
					<CommandInput placeholder="Search projects..." />
					<CommandList className="max-h-72">
						<CommandEmpty>No projects found</CommandEmpty>
						<CommandGroup>
							<CommandItem
								className={cn(value === null && "bg-surface-secondary")}
								value="no-project"
								keywords={[noProjectLabel]}
								onSelect={() => select(null)}
							>
								<MessageCircleIcon />
								<span className="truncate">{noProjectLabel}</span>
								{value === null && (
									<CheckIcon className="ml-auto size-icon-sm shrink-0" />
								)}
							</CommandItem>
						</CommandGroup>
						{isLoading ? (
							<div className="flex items-center gap-2 px-4 py-3 text-xs text-content-secondary">
								<Spinner loading size="sm" />
								Loading projects…
							</div>
						) : error ? (
							<div className="flex items-center gap-2 px-4 py-2 text-xs text-content-secondary">
								<span className="min-w-0 flex-1">
									{getErrorMessage(error, "Failed to load projects.")}
								</span>
								{onRetry && (
									<Button
										variant="subtle"
										size="sm"
										className="min-w-0"
										onClick={onRetry}
										// cmdk handles Enter on its root to select the highlighted
										// item, which would swallow the button's own activation.
										onKeyDown={(event) => event.stopPropagation()}
									>
										Retry
									</Button>
								)}
							</div>
						) : (
							options.length > 0 && (
								<CommandGroup>
									{options.map((project) => (
										<CommandItem
											key={project.id}
											className={cn(
												value?.id === project.id && "bg-surface-secondary",
											)}
											value={project.id}
											keywords={[project.name]}
											onSelect={() => select(project)}
										>
											<ChatProjectIcon project={project} className="size-4" />
											<span className="truncate">{project.name}</span>
											{value?.id === project.id && (
												<CheckIcon className="ml-auto size-icon-sm shrink-0" />
											)}
										</CommandItem>
									))}
								</CommandGroup>
							)
						)}
					</CommandList>
				</Command>
				{/* Outside Command so search never filters it out and it stays
				    visible while the list scrolls. */}
				<div className="border-0 border-t border-solid border-border px-2 py-1">
					<Button
						variant="subtle"
						size="sm"
						className="w-full justify-start gap-2 px-2 [&>svg]:p-0"
						onClick={() => {
							setOpen(false);
							onCreateProject();
						}}
					>
						<PlusIcon />
						New project
					</Button>
				</div>
			</PopoverContent>
		</Popover>
	);
};
