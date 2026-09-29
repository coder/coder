import { createContext, useContext } from "react";

// "busy" means another submission was in flight and nothing was sent; the
// caller decides whether to retry.
export type ComposerSendResult = "sent" | "busy";

export type ComposerHandle = {
	// Sends a message on the user's behalf without touching their draft.
	send: (message: string) => Promise<ComposerSendResult>;
};

/** Provides the current chat composer to right-panel tools. */
export const ComposerContext = createContext<ComposerHandle | undefined>(
	undefined,
);

export const useComposer = () => useContext(ComposerContext);
