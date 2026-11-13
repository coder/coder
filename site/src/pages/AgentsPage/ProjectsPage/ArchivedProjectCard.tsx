import { ArchiveIcon, EllipsisVerticalIcon } from "lucide-react";
import { useId, useRef } from "react";
import { Link, type To } from "react-router";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { shortRelativeTime } from "#/utils/time";
import { ChatProjectIcon } from "../components/ChatProjectIcon";

type ArchivedProjectCardProps = {
	readonly project: ChatProject;
	readonly to: To;
	readonly onUnarchive: () => void;
	readonly onDelete: (returnFocusTo: HTMLElement | null) => void;
};

/**
 * An archived project on the Projects page. The card links to the read-only
 * project page, and its actions unarchive or delete the project.
 */
export const ArchivedProjectCard: React.FC<ArchivedProjectCardProps> = ({
	project,
	to,
	onUnarchive,
	onDelete,
}) => {
	const footerId = useId();
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
	// Delete opens a dialog only once the menu has closed, so the closing
	// menu cannot take focus back from the dialog.
	const deletePickedRef = useRef(false);
	const { permissions, archived_at } = project;
	return (
		<div className="group relative flex min-w-0 flex-col gap-3 rounded-lg border bg-surface-primary p-4 hover:bg-surface-secondary has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-content-link">
			<div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-surface-secondary">
				<ChatProjectIcon project={project} className="size-4" />
			</div>
			<div className="flex flex-col gap-1">
				<h2 className="m-0 truncate text-sm font-semibold">
					<Link
						to={to}
						aria-describedby={footerId}
						// Shows the full name when it is truncated.
						title={project.name}
						// The overlay makes the whole card the link's hit area.
						className="text-content-primary no-underline outline-hidden after:absolute after:inset-0 after:rounded-lg"
					>
						{project.name}
					</Link>
				</h2>
				{project.description && (
					<p className="m-0 line-clamp-2 text-sm text-content-secondary wrap-anywhere">
						{project.description}
					</p>
				)}
			</div>
			<div
				id={footerId}
				className="mt-auto flex items-center gap-3 text-xs text-content-secondary"
			>
				<span className="flex min-w-0 items-center gap-1">
					<ArchiveIcon aria-hidden="true" className="size-3.5 shrink-0" />
					Archived
				</span>
				{archived_at && (
					<time
						dateTime={archived_at}
						data-pixel="ignore"
						className="ml-auto shrink-0 tabular-nums"
					>
						{shortRelativeTime(archived_at)}
					</time>
				)}
			</div>
			{/* Follows the link in tab order. */}
			{(permissions.archive || permissions.delete) && (
				<div className="absolute top-4 right-4 z-10 flex h-8 items-center">
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								ref={actionsButtonRef}
								variant="subtle"
								size="icon"
								aria-label={`Open project actions for ${project.name}`}
								className="size-7 max-w-0 overflow-hidden px-0 [@media(hover:none)]:max-w-7 group-hover:max-w-7 group-focus-within:max-w-7 data-[state=open]:max-w-7"
							>
								<EllipsisVerticalIcon />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent
							align="end"
							onCloseAutoFocus={(event) => {
								if (deletePickedRef.current) {
									deletePickedRef.current = false;
									event.preventDefault();
									onDelete(actionsButtonRef.current);
								}
							}}
						>
							{permissions.archive && (
								<DropdownMenuItem onSelect={onUnarchive}>
									Unarchive project
								</DropdownMenuItem>
							)}
							{permissions.delete && (
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onSelect={() => {
										deletePickedRef.current = true;
									}}
								>
									Delete project
								</DropdownMenuItem>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			)}
		</div>
	);
};
