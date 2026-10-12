import { useAutomationsEnabled } from "../Automations/automationsFlag";

const chatsInfo =
	"Chats in this project will be kept and move back to the Chats list.";

/**
 * The delete-project dialog's note on what stays. Automations keep running
 * after their project goes, so the note says where their chats start.
 */
export const useProjectDeleteInfo = (): string =>
	useAutomationsEnabled()
		? `${chatsInfo} Automations that start chats in this project keep running and start their chats outside any project.`
		: chatsInfo;
