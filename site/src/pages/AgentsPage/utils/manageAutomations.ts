import type * as TypesGen from "#/api/typesGenerated";
import { canManageChat } from "../components/ChatActionsMenuItems";

/** Whether the viewer can toggle the chat's manage_automations tool. */
export const canToggleManageAutomations = ({
	chat,
	viewerId,
	automationsExperimentEnabled,
}: {
	chat: TypesGen.Chat;
	viewerId: string;
	automationsExperimentEnabled: boolean;
}): boolean =>
	automationsExperimentEnabled &&
	canManageChat(chat, viewerId) &&
	!chat.parent_chat_id;
