import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
} from "react";

type ChatScrollElements = {
	/** The scrollport a pinned prompt measures itself against. */
	scroller: HTMLDivElement | null;
	/**
	 * The wrapper whose height follows the transcript. A growing transcript
	 * changes neither the scrollport's box nor a prompt's own box, so this is
	 * what a pinned prompt observes to keep its clip up to date.
	 */
	content: HTMLDivElement | null;
};

const ChatScrollElementsContext = createContext<ChatScrollElements>({
	scroller: null,
	content: null,
});

/**
 * Hands the scrollport and the transcript wrapper to the prompts inside them.
 * Both are elements of components above the prompt, and both are held as state
 * rather than in a ref: a prompt's layout effect runs before its ancestors'
 * refs are attached, so a ref would still be empty when the prompt first
 * measures, and nothing would tell it to look again.
 */
export const ChatScrollElementsProvider: FC<
	PropsWithChildren<ChatScrollElements>
> = ({ scroller, content, children }) => {
	return (
		<ChatScrollElementsContext.Provider value={{ scroller, content }}>
			{children}
		</ChatScrollElementsContext.Provider>
	);
};

export const useChatScrollElements = (): ChatScrollElements =>
	useContext(ChatScrollElementsContext);
