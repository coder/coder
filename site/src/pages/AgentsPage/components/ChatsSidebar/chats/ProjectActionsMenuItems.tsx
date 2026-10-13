import { SquarePenIcon } from "lucide-react";
import { Link, type To } from "react-router";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";

type ProjectActionsMenuItem = typeof DropdownMenuItem | typeof ContextMenuItem;
type ProjectActionsMenuSeparator =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

type ProjectActionsMenuItemsProps = {
	readonly Item: ProjectActionsMenuItem;
	readonly Separator: ProjectActionsMenuSeparator;
	/** Adds a "New chat" item linking here. Omit on the project page itself. */
	readonly projectPath?: To;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
};

/** Project actions shared by dropdown and context menus. */
export const ProjectActionsMenuItems: React.FC<
	ProjectActionsMenuItemsProps
> = ({ Item, Separator, projectPath, onEdit, onDelete }) => (
	<>
		{projectPath !== undefined && (
			<>
				<Item asChild>
					<Link to={projectPath}>
						<SquarePenIcon />
						New chat
					</Link>
				</Item>
				<Separator />
			</>
		)}
		<Item onSelect={onEdit}>Edit project</Item>
		<Item
			className="text-content-destructive focus:text-content-destructive"
			onSelect={onDelete}
		>
			Delete project
		</Item>
	</>
);
