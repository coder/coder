import { cn } from "cn";
import { PenIcon } from "lucide-react";
import { useId } from "react";
import type { ChatProjectInstructions } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { RetryButton } from "../RetryButton";
import { formatMembersHeading, type ProjectAccessEntry } from "./projectAccess";

/** The most avatar tiles the members row shows, including the "+K" tile. */
const MAX_MEMBER_TILES = 8;

// Long names would otherwise run off narrow screens.
const memberTooltipClassName =
	"max-w-[min(20rem,calc(100vw-2rem))] break-words";

// A button so keyboard users can focus a tile to read its tooltip.
const memberTileTriggerClassName =
	"relative flex cursor-default rounded-[6px] border-0 bg-transparent p-0 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link";

type MembersSectionProps = {
	/** `undefined` while the access list loads or after loading fails. */
	readonly entries: readonly ProjectAccessEntry[] | undefined;
	readonly error: unknown;
	readonly isRetrying: boolean;
	readonly onRetry: () => void;
	readonly onOpenAccess: (event: React.MouseEvent<HTMLButtonElement>) => void;
};

type ProjectDetailsPanelViewProps = {
	/** The owner's display label. `undefined` while it loads. */
	readonly ownerLabel: string | undefined;
	readonly createdAt: string;
	/** Omit to hide the organization row. */
	readonly organizationLabel?: string;
	/** `undefined` while the skills load or after loading fails. */
	readonly skillCount: number | undefined;
	/** `undefined` while the skills load or after loading fails. */
	readonly disabledSkillCount: number | undefined;
	readonly skillsError: unknown;
	readonly onOpenSkills: (event: React.MouseEvent<HTMLButtonElement>) => void;
	/** `undefined` until the instructions load. */
	readonly instructions: ChatProjectInstructions | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	/** Opens the instructions editor. Omit when the caller cannot edit. */
	readonly onEditInstructions?: () => void;
	/** Omit to hide the members section. */
	readonly members?: MembersSectionProps;
};

/** The project's shared configuration, shown beside its chats. */
export const ProjectDetailsPanelView: React.FC<
	ProjectDetailsPanelViewProps
> = ({
	ownerLabel,
	createdAt,
	organizationLabel,
	skillCount,
	disabledSkillCount,
	skillsError,
	onOpenSkills,
	instructions,
	error,
	onRetry,
	onEditInstructions,
	members,
}) => {
	const headingId = useId();
	const instructionsHeadingId = useId();
	const errorAlert = Boolean(error) && (
		<ErrorAlert
			error={error}
			actions={
				<Button size="sm" variant="outline" onClick={onRetry}>
					Retry
				</Button>
			}
		/>
	);
	let skillsLabel =
		skillCount === undefined
			? "Unknown"
			: skillCount === 0
				? "None"
				: skillCount === 1
					? "1 skill"
					: `${skillCount} skills`;
	if (skillCount && disabledSkillCount) {
		skillsLabel += `, ${disabledSkillCount} disabled`;
	}

	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<h2
				id={headingId}
				className="m-0 text-sm font-medium text-content-primary"
			>
				Details
			</h2>
			<div className="rounded-lg border border-border">
				<dl className="m-0 flex flex-col gap-2 border-0 border-b border-solid border-border p-4 text-sm">
					<DetailRow label="Project owner">
						{ownerLabel ?? (
							<>
								<Skeleton className="my-1 ml-auto h-3 w-12" />
								<span className="sr-only">Loading</span>
							</>
						)}
					</DetailRow>
					<DetailRow label="Created">
						<Tooltip>
							{/* A button so keyboard users can focus it to read the full time. */}
							<TooltipTrigger
								type="button"
								className="cursor-default rounded-sm border-0 bg-transparent p-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
							>
								<time dateTime={createdAt}>
									{formatDateTime(createdAt, DATE_FORMAT.MEDIUM_DATE)}
								</time>
							</TooltipTrigger>
							<TooltipContent>
								{formatDateTime(createdAt, DATE_FORMAT.FULL_DATETIME)}
							</TooltipContent>
						</Tooltip>
					</DetailRow>
					{organizationLabel !== undefined && (
						<DetailRow label="Organization">{organizationLabel}</DetailRow>
					)}
					<DetailRow label="Skills">
						{skillCount === undefined && !skillsError ? (
							<>
								<Skeleton className="my-1 ml-auto h-3 w-12" />
								<span className="sr-only">Loading</span>
							</>
						) : (
							<button
								type="button"
								aria-label={`Skills: ${skillsLabel}`}
								className="group inline-flex items-center gap-1.5 rounded-sm hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
								onClick={onOpenSkills}
							>
								{skillsLabel}
								<PenIcon className="size-3.5 shrink-0 text-content-secondary group-hover:text-content-primary" />
							</button>
						)}
					</DetailRow>
				</dl>
				<section
					aria-labelledby={instructionsHeadingId}
					className="flex flex-col gap-3 p-4"
				>
					<h3
						id={instructionsHeadingId}
						className="m-0 text-sm font-medium text-content-secondary"
					>
						Instructions
					</h3>
					{instructions === undefined ? (
						errorAlert || (
							<div className="flex flex-col gap-2 rounded-lg border border-border p-3">
								<Skeleton variant="text" className="w-full" />
								<Skeleton variant="text" className="w-2/3" />
								<span className="sr-only">Loading</span>
							</div>
						)
					) : (
						<>
							{instructions.instructions === "" ? (
								<div className="flex justify-center rounded-lg border border-dashed border-border px-4 py-6">
									{onEditInstructions ? (
										<Button
											size="sm"
											variant="outline"
											aria-label="Create instructions"
											onClick={onEditInstructions}
										>
											Create
										</Button>
									) : (
										<p className="m-0 text-sm text-content-secondary">
											No instructions
										</p>
									)}
								</div>
							) : (
								<div className="flex items-start gap-2 rounded-lg border border-border p-3">
									<p
										className={cn(
											"m-0 min-w-0 flex-1 whitespace-pre-wrap break-words text-sm text-content-primary",
											// Without the editor, this is the only place the full text shows.
											onEditInstructions && "line-clamp-3",
										)}
									>
										{instructions.instructions}
									</p>
									{onEditInstructions && (
										<Button
											size="icon"
											variant="subtle"
											aria-label="Edit instructions"
											className="-mr-1.5 -mt-1.5 shrink-0"
											onClick={onEditInstructions}
										>
											<PenIcon />
										</Button>
									)}
								</div>
							)}
							{errorAlert}
						</>
					)}
				</section>
				{members && <MembersSection {...members} />}
			</div>
		</section>
	);
};

const MembersSection: React.FC<MembersSectionProps> = ({
	entries,
	error,
	isRetrying,
	onRetry,
	onOpenAccess,
}) => {
	const headingId = useId();
	let heading = "Members";
	if (entries) {
		const groupCount = entries.filter((entry) => entry.kind === "group").length;
		heading = formatMembersHeading(entries.length - groupCount, groupCount);
	}
	// Fill the last tile with an avatar when it fits; otherwise it shows "+K".
	const visibleEntries =
		entries && entries.length > MAX_MEMBER_TILES
			? entries.slice(0, MAX_MEMBER_TILES - 1)
			: entries;
	const hiddenEntries = entries?.slice(visibleEntries?.length ?? 0) ?? [];
	const isLoading = !entries && !error;

	return (
		<section
			aria-labelledby={headingId}
			className="flex flex-col gap-3 border-0 border-t border-solid border-border p-4"
		>
			<div className="flex items-center justify-between gap-4">
				<h3
					id={headingId}
					className={cn(
						"m-0 text-sm font-medium text-content-secondary",
						isLoading && "sr-only",
					)}
				>
					{heading}
				</h3>
				{isLoading && <Skeleton className="h-3 w-24" />}
				<Button size="sm" variant="outline" onClick={onOpenAccess}>
					View access
				</Button>
			</div>
			{visibleEntries && (
				<ul aria-label="Project members" className="m-0 flex list-none p-0">
					{visibleEntries.map((entry) => (
						<li
							key={`${entry.kind}:${entry.id}`}
							className="flex rounded-[6px] ring-2 ring-surface-primary not-first:-ml-2"
						>
							<MemberAvatar entry={entry} />
						</li>
					))}
					{hiddenEntries.length > 0 && (
						<li className="-ml-2 flex rounded-[6px] ring-2 ring-surface-primary">
							<Tooltip>
								<TooltipTrigger
									type="button"
									aria-label={`${hiddenEntries.length} more`}
									className={memberTileTriggerClassName}
								>
									<span className="flex size-(--avatar-lg) items-center justify-center rounded-[6px] border border-solid border-border bg-surface-secondary text-sm font-medium text-content-secondary">
										+{hiddenEntries.length}
									</span>
								</TooltipTrigger>
								<TooltipContent className={memberTooltipClassName}>
									{hiddenEntries.map((entry) => entry.name).join(", ")}
								</TooltipContent>
							</Tooltip>
						</li>
					)}
				</ul>
			)}
			{isLoading && (
				<div className="flex">
					{Array.from({ length: 3 }, (_, index) => (
						<Skeleton
							key={index}
							className="size-(--avatar-lg) rounded-[6px] ring-2 ring-surface-primary not-first:-ml-2"
						/>
					))}
					<span className="sr-only">Loading</span>
				</div>
			)}
			{Boolean(error) && (
				<ErrorAlert
					error={error}
					showDebugDetail={false}
					actions={
						<RetryButton
							size="sm"
							variant="outline"
							isRetrying={isRetrying}
							onRetry={onRetry}
						/>
					}
				/>
			)}
		</section>
	);
};

type MemberAvatarProps = {
	readonly entry: ProjectAccessEntry;
};

const MemberAvatar: React.FC<MemberAvatarProps> = ({ entry }) => {
	if (entry.name === undefined) {
		return (
			<>
				<Skeleton className="size-(--avatar-lg) rounded-[6px]" />
				<span className="sr-only">Loading</span>
			</>
		);
	}
	return (
		<Tooltip>
			<TooltipTrigger
				type="button"
				aria-label={entry.name}
				className={memberTileTriggerClassName}
			>
				<Avatar
					size="lg"
					src={entry.avatarUrl}
					fallback={entry.name}
					aria-hidden="true"
				/>
			</TooltipTrigger>
			<TooltipContent className={memberTooltipClassName}>
				{entry.name}
			</TooltipContent>
		</Tooltip>
	);
};

type DetailRowProps = {
	readonly label: string;
	readonly children: React.ReactNode;
};

const DetailRow: React.FC<DetailRowProps> = ({ label, children }) => (
	<div className="flex items-start justify-between gap-4">
		<dt className="shrink-0 text-content-secondary">{label}</dt>
		<dd className="m-0 min-w-0 break-words text-right font-mono text-content-primary">
			{children}
		</dd>
	</div>
);
