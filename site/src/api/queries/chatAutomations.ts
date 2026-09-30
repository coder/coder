import { API } from "#/api/api";
import type { ChatAutomation } from "#/api/typesGenerated";

const chatAutomationsKey = (organizationId: string) =>
	["organizations", organizationId, "chat-automations"] as const;

export const chatAutomations = (organizationId: string) => ({
	queryKey: chatAutomationsKey(organizationId),
	queryFn: (): Promise<ChatAutomation[]> =>
		API.experimental.getChatAutomations(organizationId),
	enabled: organizationId !== "",
});
