import { createContext, useContext } from "react";

type ArtifactTabsContextValue = {
	/**
	 * Opens the artifact in its own right panel tab, at `version`, or at the
	 * latest version when it is unset. An open tab of the artifact is reused.
	 */
	onOpenArtifact?: (
		artifactId: string,
		title: string,
		version?: number,
	) => void;
};

export const ArtifactTabsContext = createContext<ArtifactTabsContextValue>({});

export const useArtifactTabs = () => useContext(ArtifactTabsContext);
