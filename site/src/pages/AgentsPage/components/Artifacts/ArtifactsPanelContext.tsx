import { createContext, useContext } from "react";

type ArtifactsPanelContextValue = {
	/** Opens the right panel on the Artifacts tab with this version selected. */
	onOpenArtifact?: (artifactId: string, version: number) => void;
};

export const ArtifactsPanelContext = createContext<ArtifactsPanelContextValue>(
	{},
);

export const useArtifactsPanel = () => useContext(ArtifactsPanelContext);
