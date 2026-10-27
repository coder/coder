import { cn } from "cn";
import { ChevronDownIcon } from "lucide-react";
import { Slot } from "radix-ui";
import { useEffect, useRef } from "react";
import { Link, type Location } from "react-router";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { shortRelativeTime } from "#/utils/time";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatProjectIcon } from "../../ChatProjectIcon";
import { ChatNodePRIcon } from "../tree/ChatNodePRIcon";
import { getChatDisplayConfig } from "../tree/statusConfig";

type SearchOptionProps = {
	readonly id: string;
	readonly isSelected: boolean;
	readonly className: string;
	readonly children: React.ReactElement;
};

/**
 * Renders its child element as a listbox option. The search input keeps focus
 * and selects options through aria-activedescendant, so the selected option
 * scrolls itself into view.
 */
const SearchOption: React.FC<SearchOptionProps> = ({
	id,
	isSelected,
	className,
	children,
}) => {
	const optionRef = useRef<HTMLElement | null>(null);

	useEffect(() => {
		if (isSelected) {
			optionRef.current?.scrollIntoView({ block: "nearest" });
		}
	}, [isSelected]);

	return (
		<Slot.Root
			ref={optionRef}
			id={id}
			role="option"
			aria-selected={isSelected}
			tabIndex={-1}
			className={cn(
				"w-full gap-2 rounded-md px-1.5 py-1 text-content-secondary no-underline hover:bg-surface-tertiary/40 hover:text-content-primary aria-selected:bg-surface-tertiary/40 aria-selected:text-content-primary",
				className,
			)}
		>
			{children}
		</Slot.Root>
	);
};

type ProjectSearchResultRowProps = {
	readonly project: ChatProject;
	readonly organizationLabel: string | undefined;
	readonly id: string;
	readonly isSelected: boolean;
	readonly location: Location;
	readonly onSelect: () => void;
};

/** A project result that opens the project page. */
export const ProjectSearchResultRow: React.FC<ProjectSearchResultRowProps> = ({
	project,
	organizationLabel,
	id,
	isSelected,
	location,
	onSelect,
}) => (
	<SearchOption id={id} isSelected={isSelected} className="flex items-center">
		<Link
			to={{
				pathname: buildAgentProjectPath({ projectId: project.id }),
				search: location.search,
			}}
			onClick={onSelect}
		>
			<ChatProjectIcon project={project} className="size-3.5" />
			<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
				{project.name}
			</span>
			{organizationLabel && (
				<span className="max-w-2/5 shrink-0 truncate text-xs">
					{organizationLabel}
				</span>
			)}
		</Link>
	</SearchOption>
);

type ShowAllProjectsRowProps = {
	readonly projectCount: number;
	readonly id: string;
	readonly isSelected: boolean;
	readonly onShowAll: () => void;
};

/** Expands a collapsed project list in place. */
export const ShowAllProjectsRow: React.FC<ShowAllProjectsRowProps> = ({
	projectCount,
	id,
	isSelected,
	onShowAll,
}) => (
	<SearchOption
		id={id}
		isSelected={isSelected}
		className="flex items-center text-left text-sm"
	>
		<button
			type="button"
			// Keeps focus in the search input so typing still works after a click.
			onMouseDown={(event) => event.preventDefault()}
			onClick={onShowAll}
		>
			<ChevronDownIcon aria-hidden="true" className="size-3.5 shrink-0" />
			Show all {projectCount} projects
		</button>
	</SearchOption>
);

type ChatSearchResultRowProps = {
	readonly chat: Chat;
	readonly id: string;
	readonly isSelected: boolean;
	readonly location: Location;
	readonly onSelect: () => void;
};

/** A chat result: status, title, PR stats, latest summary, and age. */
export const ChatSearchResultRow: React.FC<ChatSearchResultRowProps> = ({
	chat,
	id,
	isSelected,
	location,
	onSelect,
}) => {
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
		prStatuses,
	} = getChatDisplayConfig(chat);
	// Line stats only make sense for one PR; several PRs have no
	// single set of counts, and the icon shows the count instead.
	const solePR = prStatuses.length === 1 ? prStatuses[0] : undefined;

	const additions = solePR?.additions ?? 0;
	const deletions = solePR?.deletions ?? 0;
	const changedFiles = solePR?.changed_files ?? 0;
	const hasLineStats =
		Boolean(solePR?.url) &&
		(additions > 0 || deletions > 0 || changedFiles > 0);
	const subtitle = chat.last_turn_summary?.trim() || "No summary available";

	return (
		<SearchOption
			id={id}
			isSelected={isSelected}
			className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-start"
		>
			<Link
				to={{ pathname: `/agents/${chat.id}`, search: location.search }}
				onClick={onSelect}
			>
				<StatusIcon
					role="img"
					aria-label={statusLabel}
					className={cn("mt-1 size-3.5 shrink-0", statusClassName)}
				/>
				<div className="min-w-0">
					<div className="truncate text-sm text-content-primary">
						{chat.title}
					</div>
					<div className="flex min-w-0 items-center gap-1.5 text-xs">
						<ChatNodePRIcon prStatuses={prStatuses} />
						{hasLineStats && (
							<span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums">
								<span className="text-git-added-bright">+{additions}</span>
								<span className="text-git-deleted-bright">
									&minus;{deletions}
								</span>
							</span>
						)}
						<span className="min-w-0 flex-1 truncate text-content-secondary">
							{subtitle}
						</span>
					</div>
				</div>
				<span className="inline-flex shrink-0 items-center gap-1.5 pt-0.5 text-xs text-content-secondary">
					{chat.has_unread && (
						<span
							className="size-1.5 shrink-0 rounded-full bg-content-link"
							aria-hidden="true"
						/>
					)}
					{/* Pin the ignored mask width so Pixel does not diff bounding rect changes. */}
					<span data-pixel="ignore" className="inline-block w-7 text-right">
						{shortRelativeTime(chat.updated_at)}
					</span>
				</span>
			</Link>
		</SearchOption>
	);
};

/** Placeholder rows while the first chat search loads. */
export const ChatSearchResultsSkeleton: React.FC = () => (
	<div className="space-y-1.5">
		{Array.from({ length: 6 }, (_, index) => (
			<div
				key={`search-skeleton-${index}`}
				className="flex items-start gap-2 rounded-md px-1.5 py-1"
			>
				<Skeleton className="mt-1 size-3.5 shrink-0 rounded-full" />
				<div className="min-w-0 flex-1 space-y-1.5">
					<Skeleton
						className="h-3.5"
						style={{ width: `${60 + ((index * 11) % 30)}%` }}
					/>
					<Skeleton
						className="h-3"
						style={{ width: `${50 + ((index * 13) % 35)}%` }}
					/>
				</div>
				<Skeleton className="h-3 w-6" />
			</div>
		))}
	</div>
);
