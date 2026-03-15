import { createContext, useContext } from "react";

interface EmbedContextValue {
	isEmbedded: boolean;
}

const EmbedContext = createContext<EmbedContextValue>({
	isEmbedded: false,
});

export const EmbedProvider = EmbedContext.Provider;

export const useEmbedContext = () => useContext(EmbedContext);
