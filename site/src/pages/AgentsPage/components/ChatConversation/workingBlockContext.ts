import { createContext, useContext } from "react";

/**
 * True for rows rendered inside an expanded working block, so nested
 * renderers can adapt to the block's rule (for example, bracketing
 * narration off it).
 */
export const WorkingBlockContext = createContext(false);

export const useInsideWorkingBlock = (): boolean =>
	useContext(WorkingBlockContext);
