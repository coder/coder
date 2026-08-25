import { generateConnectionSessionId } from "#/utils/random";

/**
 * Maps a terminal's reconnectionToken to a client session ID that stays stable
 * for the lifetime of the page. Because this registry lives in module memory it
 * survives TerminalPanel unmount/remount (tab hide/show, switching chats) but
 * resets on a full page reload. Reattaching to a live PTY therefore keeps the
 * same session ID, while a reload starts a new one, per the connection-log RFC.
 */
const clientSessionIdsByReconnectionToken = new Map<string, string>();

/**
 * Returns the client session ID for a terminal reconnection token, generating
 * and storing one on first use. The same token always resolves to the same ID
 * until the page reloads.
 */
export function getTerminalClientSessionId(reconnectionToken: string): string {
	let sessionId = clientSessionIdsByReconnectionToken.get(reconnectionToken);
	if (!sessionId) {
		sessionId = generateConnectionSessionId();
		clientSessionIdsByReconnectionToken.set(reconnectionToken, sessionId);
	}
	return sessionId;
}

/** Clears the registry. Test-only, to keep session IDs from leaking between cases. */
export function resetTerminalClientSessionIds(): void {
	clientSessionIdsByReconnectionToken.clear();
}
