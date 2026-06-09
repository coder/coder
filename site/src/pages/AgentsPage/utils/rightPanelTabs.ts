export type UserRightPanelTab = {
	id: string;
	kind: "terminal";
	/**
	 * PTY reconnect token. Must be a UUID; the backend rejects non-UUIDs.
	 * Persisting it keeps the session attached across reloads.
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
