import { API } from "#/api/api";
import type { ChatProject } from "#/api/typesGenerated";

const chatProjectsKey = ["chat-projects"] as const;

export const chatProjects = () => ({
	queryKey: chatProjectsKey,
	queryFn: (): Promise<ChatProject[]> => API.experimental.getChatProjects(),
});
