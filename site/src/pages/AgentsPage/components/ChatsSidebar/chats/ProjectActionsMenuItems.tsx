import { SquarePenIcon } from "lucide-react";
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
	/** Adds a "Pin" item. */
	readonly onPin?: () => void;
	/** Adds an "Unpin" item. */
	readonly onUnpin?: () => void;
	readonly project: Pick<ChatProject, "permissions" | "archived_at">;
	/** When omitted, an active project gets no "Edit" item. */
	readonly onEdit?: () => void;
	/** When omitted, an active project gets no "Archive" item. */
	readonly onArchive?: () => void;
	/** When omitted, an archived project gets no "Unarchive" item. */
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
	const showEdit = onEdit && permissions.update && !isArchived;
	const showArchive = onArchive && permissions.archive && !isArchived;
	const showUnarchive = onUnarchive && permissions.archive && isArchived;
	const hasItemsAboveDelete =
		onPin || onUnpin || showEdit || showArchive || showUnarchive;
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
			{onPin && <Item onSelect={onPin}>Pin</Item>}
			{onUnpin && <Item onSelect={onUnpin}>Unpin</Item>}
			{showEdit && <Item onSelect={onEdit}>Edit</Item>}
			{showArchive && <Item onSelect={onArchive}>Archive</Item>}
			{showUnarchive && <Item onSelect={onUnarchive}>Unarchive</Item>}
			{permissions.delete && (
				<>
					{hasItemsAboveDelete && <Separator />}
					<Item
						className="text-content-destructive focus:text-content-destructive"
						onSelect={onDelete}
					>
						Delete
					</Item>
				</>
			)}
		</>
	);
};
