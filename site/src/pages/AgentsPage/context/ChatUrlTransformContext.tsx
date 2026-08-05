import { createContext, useContext } from "react";

/** Single-argument shape stays assignable to Streamdown's urlTransform. */
export type ChatUrlTransform = (url: string) => string;

const ChatUrlTransformContext = createContext<ChatUrlTransform | undefined>(
	undefined,
);

export const useChatUrlTransform = () => useContext(ChatUrlTransformContext);

export { ChatUrlTransformContext };
