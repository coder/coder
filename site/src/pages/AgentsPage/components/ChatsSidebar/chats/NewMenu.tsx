import { ChevronDownIcon, PlusIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import { Link } from "react-router";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { navItemClassName } from "../settings/SettingsNavItem";

interface NewMenuProps {
	readonly active: boolean;
	readonly disabled?: boolean;
	readonly newChatTo: ComponentProps<typeof Link>["to"];
	readonly onNewChat?: () => void;
	readonly onNewProject?: () => void;
}

/**
 * NewMenu is the sidebar's "New" entry point. It opens a menu with the
 * things a user can create from the agents page: a chat or a project.
 */
export const NewMenu: FC<NewMenuProps> = ({
	active,
	disabled,
	newChatTo,
	onNewChat,
	onNewProject,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					disabled={disabled}
					className={navItemClassName(
						active,
						disabled,
						"group data-[state=open]:text-content-primary",
					)}
				>
					<PlusIcon className="size-4 shrink-0" />
					<span className="min-w-0">New</span>
					<ChevronDownIcon className="size-3.5 shrink-0 transition-transform group-data-[state=open]:rotate-180" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="w-[260px]">
				<DropdownMenuItem asChild>
					<Link to={newChatTo} onClick={onNewChat}>
						New chat
					</Link>
				</DropdownMenuItem>
				<DropdownMenuItem onSelect={onNewProject}>New project</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
