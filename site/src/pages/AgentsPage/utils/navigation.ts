const safeChatIdPattern = /^[A-Za-z0-9._~-]+$/;
const recoverableChatIdPrefixPattern = /^([A-Za-z0-9._~-]+)[?\s]/;

export const buildAgentChatPath = ({
	chatId,
}: Readonly<{
	chatId: string;
}>): string => {
	return `/agents/${encodeURIComponent(chatId)}`;
};

export const safeBuildAgentChatPath = ({
	chatId,
}: Readonly<{
	chatId: string;
}>): string | null => {
	const trimmedChatId = chatId.trim();
	if (safeChatIdPattern.test(trimmedChatId)) {
		return buildAgentChatPath({ chatId: trimmedChatId });
	}

	const recoverableChatIdPrefix = trimmedChatId.match(
		recoverableChatIdPrefixPattern,
	)?.[1];
	if (!recoverableChatIdPrefix) {
		return null;
	}

	return buildAgentChatPath({ chatId: recoverableChatIdPrefix });
};

/**
 * Router state carried from "New chat here" to the composer. The parent
 * is not a search param so it never leaks into links that copy
 * location.search.
 */
export type NewChildChatLocationState = {
	readonly parentChatId: string;
	readonly parentChatTitle: string;
	readonly parentOrganizationId: string;
};

export const readNewChildChatLocationState = (
	state: unknown,
): NewChildChatLocationState | undefined => {
	if (!state || typeof state !== "object") {
		return undefined;
	}
	const record = state as Record<string, unknown>;
	if (
		typeof record.parentChatId !== "string" ||
		typeof record.parentChatTitle !== "string" ||
		typeof record.parentOrganizationId !== "string"
	) {
		return undefined;
	}
	return {
		parentChatId: record.parentChatId,
		parentChatTitle: record.parentChatTitle,
		parentOrganizationId: record.parentOrganizationId,
	};
};
