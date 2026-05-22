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
