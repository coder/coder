import { Share2Icon } from "lucide-react";
import { type FC, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { ChatSharingForm } from "./ChatSharingForm";
import { useChatSharing } from "./useChatSharing";

type ChatSharePopoverProps = {
	chat: Chat;
	canShare: boolean;
};

export const ChatSharePopover: FC<ChatSharePopoverProps> = ({
	chat,
	canShare,
}) => {
	const [open, setOpen] = useState(false);
	const sharing = useChatSharing(chat);

	// The backend emits 403 when org or deployment kill-switches disable
	// sharing. We still render the popover so the owner can see existing
	// ACL state, but the form's error region will show the 403.
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="sm"
					className="h-7 gap-1.5 px-2 text-content-secondary hover:text-content-primary"
					aria-label="Share chat"
					data-testid="chat-share-button"
				>
					<Share2Icon className="size-icon-xs" />
					Share
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-[640px] p-4">
				<div className="flex items-center gap-2 mb-4">
					<h3 className="text-sm font-semibold m-0">Share chat</h3>
				</div>
				<ChatSharingForm
					organizationId={chat.organization_id}
					chatACL={sharing.chatACL}
					canUpdatePermissions={canShare}
					error={sharing.error ?? sharing.mutationError}
					isMutating={sharing.isMutating}
					updatingUserId={sharing.updatingUserId}
					updatingGroupId={sharing.updatingGroupId}
					onAddUser={sharing.addUser}
					onAddGroup={sharing.addGroup}
					onRemoveUser={sharing.removeUser}
					onRemoveGroup={sharing.removeGroup}
					onUpdateUserEntry={sharing.setUserEntry}
					onUpdateGroupEntry={sharing.setGroupEntry}
				/>
			</PopoverContent>
		</Popover>
	);
};
