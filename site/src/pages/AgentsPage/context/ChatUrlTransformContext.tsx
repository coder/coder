import { createContext, useContext } from "react";

/**
 * The single-argument shape (vs streamdown's UrlTransform) lets
 * plain-text surfaces call it with just a URL while staying assignable
 * to Streamdown's urlTransform prop.
 */
export type ChatUrlTransform = (url: string) => string;

const ChatUrlTransformContext = createContext<ChatUrlTransform | undefined>(
	undefined,
);

export const useChatUrlTransform = () => useContext(ChatUrlTransformContext);

export { ChatUrlTransformContext };
