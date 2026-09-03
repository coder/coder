import { createContext, useContext } from "react";

const TerminalClientSessionContext = createContext<string | undefined>(
	undefined,
);

/**
 * Returns the client session ID for the current agent chat page visit. It is
 * generated once when the page mounts and stays stable until the page unmounts
 * (navigating to another chat, leaving the page, or reloading), independent of
 * any terminal's reconnection token. Must be used within a
 * TerminalClientSessionContext provider.
 */
export const useTerminalClientSessionId = (): string => {
	const sessionId = useContext(TerminalClientSessionContext);
	if (sessionId === undefined) {
		throw new Error(
			"useTerminalClientSessionId must be used within a TerminalClientSessionContext provider",
		);
	}
	return sessionId;
};

export { TerminalClientSessionContext };
