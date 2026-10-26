import { createContext } from "react";

/** The async question call shown in the dock above the chat input. */
export const DockedAskUserQuestionContext = createContext<string | undefined>(
	undefined,
);
