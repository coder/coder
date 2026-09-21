import { API } from "../api";
import type { ACPMessageRequest, ACPSession } from "../typesGenerated";

const acpKey = "acp-sessions";

export const acpSessionPath = (
	parent: string,
	agent: string,
	session: string,
) =>
	`/api/v2/chats/${encodeURIComponent(parent)}/acp/agents/${encodeURIComponent(agent)}/sessions/${encodeURIComponent(session)}`;

export const acpSession = (path: string) => ({
	queryKey: [acpKey, path],
	queryFn: () => API.getACPSession(path),
	retry: false,
	isDataEqual: (
		previous: ACPSession | null | undefined,
		next: ACPSession | null,
	) => Boolean(previous && next && previous.version > next.version),
});

export const sendACPMessage = (path: string) => ({
	mutationFn: (request: ACPMessageRequest) => API.sendACPMessage(path, request),
});

export const interruptACPSession = (path: string) => ({
	mutationFn: () => API.interruptACPSession(path),
});
