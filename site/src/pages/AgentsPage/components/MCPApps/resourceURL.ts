/** Uses a document URL to allow an app-specific response CSP. */
export function mcpAppResourceURL(
	chatId: string,
	server: string,
	uri: string,
): string {
	return (
		"/api/v2/chats/" +
		encodeURIComponent(chatId) +
		"/mcp-apps/resource?" +
		new URLSearchParams({ server, uri })
	);
}
