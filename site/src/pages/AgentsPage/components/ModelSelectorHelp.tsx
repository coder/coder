import type { ReactNode } from "react";
import { AgentSettingsLink } from "./AgentSettingsDialog/AgentSettingsLink";

type GetModelSelectorHelpOptions = {
	isModelCatalogLoading: boolean;
	hasModelOptions: boolean;
	hasConfiguredModels: boolean;
	hasUserFixableModelProviders: boolean;
};

export const getModelSelectorHelp = ({
	isModelCatalogLoading,
	hasModelOptions,
	hasConfiguredModels,
	hasUserFixableModelProviders,
}: GetModelSelectorHelpOptions): ReactNode | undefined => {
	if (
		isModelCatalogLoading ||
		hasModelOptions ||
		!hasConfiguredModels ||
		!hasUserFixableModelProviders
	) {
		return undefined;
	}

	return (
		<>
			Configure your API keys in{" "}
			<AgentSettingsLink
				section="api-keys"
				className="underline transition-colors hover:text-content-primary"
			>
				Settings
			</AgentSettingsLink>{" "}
			to enable models.
		</>
	);
};
