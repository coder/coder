import { createContext } from "react";
import type { AsyncQuestion } from "../../ChatConversation/asyncQuestions";

type AsyncQuestionsContextValue = {
	byId: ReadonlyMap<string, AsyncQuestion>;
	/** The ID of the question that the question panel shows, if any. */
	panelQuestionId: string | undefined;
};

export const AsyncQuestionsContext = createContext<AsyncQuestionsContextValue>({
	byId: new Map(),
	panelQuestionId: undefined,
});
