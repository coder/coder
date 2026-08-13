import { createContext, useContext } from "react";

export type ChatUrlTransform = (url: string) => string;

const ChatUrlTransformContext = createContext<ChatUrlTransform | undefined>(
	undefined,
);

export const useChatUrlTransform = () => useContext(ChatUrlTransformContext);

export { ChatUrlTransformContext };
