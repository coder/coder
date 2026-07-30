// generateSessionId returns a 16-byte session identifier encoded as a
// 32-character hexadecimal string, as required by the connection-log
// correlation RFC.
export const generateSessionId = (): string => {
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
		"",
	);
};
