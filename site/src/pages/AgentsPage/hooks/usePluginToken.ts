import { useCallback, useEffect, useRef, useState } from "react";
import { API } from "#/api/api";

// Go's time.Duration is in nanoseconds when serialized to JSON.
const NS_PER_MS = 1_000_000;
const PLUGIN_TOKEN_LIFETIME_MS = 60 * 60 * 1000; // 1 hour
const PLUGIN_TOKEN_REFRESH_BUFFER_MS = 5 * 60 * 1000; // Refresh 5 min before expiry
const REFRESH_INTERVAL_MS =
	PLUGIN_TOKEN_LIFETIME_MS - PLUGIN_TOKEN_REFRESH_BUFFER_MS;

interface UsePluginTokenResult {
	token: string | null;
	isLoading: boolean;
	error: Error | null;
	refresh: () => Promise<string | null>;
}

/**
 * Mints a short-lived Coder API token for a plugin iframe.
 * The token is created on first activation and automatically
 * refreshed before expiry. Cleanup happens on unmount.
 */
export function usePluginToken(
	pluginSlug: string,
	chatId: string,
	isActive: boolean,
): UsePluginTokenResult {
	const [token, setToken] = useState<string | null>(null);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState<Error | null>(null);
	const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const createToken = useCallback(async (): Promise<string | null> => {
		try {
			setIsLoading(true);
			setError(null);
			const tokenName = `plugin-${pluginSlug}-${chatId}-${Date.now()}`;
			const resp = await API.createToken({
				lifetime: PLUGIN_TOKEN_LIFETIME_MS * NS_PER_MS,
				scope: "all",
				token_name: tokenName,
			});
			setToken(resp.key);
			setIsLoading(false);
			return resp.key;
		} catch (e) {
			setError(e instanceof Error ? e : new Error(String(e)));
			setIsLoading(false);
			return null;
		}
	}, [pluginSlug, chatId]);

	// Create initial token when activated.
	useEffect(() => {
		if (isActive && !token && !isLoading) {
			void createToken();
		}
	}, [isActive, token, isLoading, createToken]);

	// Schedule periodic refresh.
	useEffect(() => {
		if (!isActive || !token) {
			return;
		}
		refreshTimerRef.current = setTimeout(() => {
			void createToken();
		}, REFRESH_INTERVAL_MS);

		return () => {
			if (refreshTimerRef.current) {
				clearTimeout(refreshTimerRef.current);
			}
		};
	}, [isActive, token, createToken]);

	return { token, isLoading, error, refresh: createToken };
}
