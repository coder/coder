export const buildAgentChatPath = ({
	chatId,
}: Readonly<{
	chatId: string;
}>): string => {
	return `/agents/${chatId}`;
};
