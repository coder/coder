import { GitPullRequestIcon } from "lucide-react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { PRMenuLinks, prMenuContentClassName } from "../../PRMenuLinks";

type ChatPRMenuItemsProps = {
	readonly prStatuses: readonly ChatDiffStatus[];
	readonly Item: typeof DropdownMenuItem | typeof ContextMenuItem;
	readonly Sub: typeof DropdownMenuSub | typeof ContextMenuSub;
	readonly SubTrigger:
		| typeof DropdownMenuSubTrigger
		| typeof ContextMenuSubTrigger;
	readonly SubContent:
		| typeof DropdownMenuSubContent
		| typeof ContextMenuSubContent;
};

export const ChatPRMenuItems: React.FC<ChatPRMenuItemsProps> = ({
	prStatuses,
	Item,
	Sub,
	SubTrigger,
	SubContent,
}) => {
	const links = <PRMenuLinks prStatuses={prStatuses} Item={Item} />;

	if (prStatuses.length === 1) {
		return links;
	}

	return (
		<Sub>
			<SubTrigger>
				<GitPullRequestIcon />
				{prStatuses.length} PRs
			</SubTrigger>
			<SubContent className={prMenuContentClassName}>{links}</SubContent>
		</Sub>
	);
};
