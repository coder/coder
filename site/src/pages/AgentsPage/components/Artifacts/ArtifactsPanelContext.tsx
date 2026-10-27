import { createContext, useContext } from "react";

type ArtifactsPanelContextValue = {
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

export const ArtifactsPanelContext = createContext<ArtifactsPanelContextValue>(
	{},
);

export const useArtifactsPanel = () => useContext(ArtifactsPanelContext);
