import { createContext, useContext } from "react";

/** True for rows rendered inside an expanded working block. */
export const WorkingBlockContext = createContext(false);

export const useInsideWorkingBlock = (): boolean =>
	useContext(WorkingBlockContext);
