import { createContext } from "react";

export const ExperimentalFormContext = createContext<
	{ toggleOptedOut: () => void } | undefined
>(undefined);
