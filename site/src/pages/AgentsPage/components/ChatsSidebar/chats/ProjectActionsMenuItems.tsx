import { PinIcon, PinOffIcon, SquarePenIcon } from "lucide-react";
import { Link, type To } from "react-router";
import type { ChatProject } from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import { highlightComposerState } from "../../../utils/composerHighlight";

type ProjectActionsMenuItem = typeof DropdownMenuItem | typeof ContextMenuItem;
type ProjectActionsMenuSeparator =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

type ProjectActionsMenuItemsProps = {
	readonly Item: ProjectActionsMenuItem;
	readonly Separator: ProjectActionsMenuSeparator;
	/** Adds a "New chat" item linking here. Omit on the project page itself. */
	readonly projectPath?: To;
	readonly onNewChatInPlace?: () => void;
	/** Adds a "Pin project" item. */
	readonly onPin?: () => void;
	/** Adds an "Unpin project" item. */
	readonly onUnpin?: () => void;
	readonly project: Pick<ChatProject, "permissions" | "archived_at">;
	readonly onEdit: () => void;
	readonly onArchive: () => void;
	/** When omitted, an archived project gets no "Unarchive project" item. */
	readonly onUnarchive?: () => void;
	readonly onDelete: () => void;
};

/** Reports whether ProjectActionsMenuItems renders any permission-gated item. */
export const hasProjectManageAction = ({
	permissions,
	archived_at,
}: Pick<ChatProject, "permissions" | "archived_at">) =>
	(permissions.update && !archived_at) ||
	permissions.archive ||
	permissions.delete;

/** Project actions shared by dropdown and context menus. */
export const ProjectActionsMenuItems: React.FC<
	ProjectActionsMenuItemsProps
> = ({
	Item,
	Separator,
	projectPath,
	onNewChatInPlace,
	onPin,
	onUnpin,
	project,
	onEdit,
	onArchive,
	onUnarchive,
	onDelete,
}) => {
	const { permissions } = project;
	const isArchived = Boolean(project.archived_at);
	return (
		<>
			{projectPath !== undefined && (
				<>
					<Item asChild>
						<Link
							to={projectPath}
							state={onNewChatInPlace ? highlightComposerState : undefined}
							onClick={onNewChatInPlace}
						>
							<SquarePenIcon />
							New chat
						</Link>
					</Item>
					{(onPin || onUnpin || hasProjectManageAction(project)) && (
						<Separator />
					)}
				</>
			)}
			{onPin && (
				<Item onSelect={onPin}>
					<PinIcon />
					Pin project
				</Item>
			)}
			{onUnpin && (
				<Item onSelect={onUnpin}>
					<PinOffIcon />
					Unpin project
				</Item>
			)}
			{permissions.update && !isArchived && (
				<Item onSelect={onEdit}>Edit project</Item>
			)}
			{permissions.archive &&
				(isArchived ? (
					onUnarchive && <Item onSelect={onUnarchive}>Unarchive project</Item>
				) : (
					<Item onSelect={onArchive}>Archive project</Item>
				))}
			{permissions.delete && (
				<Item
					className="text-content-destructive focus:text-content-destructive"
					onSelect={onDelete}
				>
					Delete project
				</Item>
			)}
		</>
	);
};
