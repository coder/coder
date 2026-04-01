import { createContext, useContext } from "react";
import type { StickyEntry } from "./ChatConversation/useStickyScrollHandler";

/**
 * Provides the shared sticky scroll handler's register/unregister
 * callbacks to StickyUserMessage instances via context, avoiding
 * prop drilling through the component tree.
 */
interface StickyScrollContextValue {
	register: (entry: StickyEntry) => void;
	unregister: (entry: StickyEntry) => void;
}

const noop = () => {};

export const StickyScrollContext = createContext<StickyScrollContextValue>({
	register: noop,
	unregister: noop,
});

export function useStickyScrollContext(): StickyScrollContextValue {
	return useContext(StickyScrollContext);
}
