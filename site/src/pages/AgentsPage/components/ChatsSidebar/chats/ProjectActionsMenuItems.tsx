import { PinIcon, PinOffIcon, SquarePenIcon } from "lucide-react";
import { Link, type To } from "react-router";
import type { ChatProjectPermissions } from "#/api/typesGenerated";
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
	/** The user's pin, shown with onTogglePin as Pin or Unpin project. */
	readonly pinned?: boolean;
	readonly onTogglePin?: () => void;
	readonly permissions: ChatProjectPermissions;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
};

/** Reports whether ProjectActionsMenuItems renders any permission-gated item. */
export const hasProjectUpdateOrDeleteAction = (
	permissions: ChatProjectPermissions,
) => permissions.update || permissions.delete;

/** Project actions shared by dropdown and context menus. */
export const ProjectActionsMenuItems: React.FC<
	ProjectActionsMenuItemsProps
> = ({
	Item,
	Separator,
	projectPath,
	onNewChatInPlace,
	pinned = false,
	onTogglePin,
	permissions,
	onEdit,
	onDelete,
}) => (
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
				{(onTogglePin || hasProjectUpdateOrDeleteAction(permissions)) && (
					<Separator />
				)}
			</>
		)}
		{onTogglePin && (
			<Item onSelect={onTogglePin}>
				{pinned ? <PinOffIcon /> : <PinIcon />}
				{pinned ? "Unpin project" : "Pin project"}
			</Item>
		)}
		{permissions.update && <Item onSelect={onEdit}>Edit project</Item>}
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
