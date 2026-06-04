export type UserRightPanelTab = {
	id: string;
	kind: "terminal";
	/**
	 * UUID used as the PTY reconnect token. The backend rejects
	 * reconnect tokens that are not valid UUIDs, so each terminal tab
	 * stores its own generated UUID. Persisting it keeps the PTY
	 * session attached across reloads.
	 */
	reconnectionToken: string;
};

export function isUserRightPanelTab(
	value: unknown,
): value is UserRightPanelTab {
	if (typeof value !== "object" || value === null) {
		return false;
	}
	const record = value as Record<string, unknown>;
	if (typeof record.id !== "string") {
		return false;
	}

	if (record.kind === "terminal") {
		return typeof record.reconnectionToken === "string";
	}

	return false;
}
