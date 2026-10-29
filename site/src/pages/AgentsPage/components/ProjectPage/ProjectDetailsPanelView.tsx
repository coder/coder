import { cn } from "cn";
import { PenIcon } from "lucide-react";
import { useId } from "react";
import type { ChatProjectInstructions } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

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
			</div>
		</section>
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
