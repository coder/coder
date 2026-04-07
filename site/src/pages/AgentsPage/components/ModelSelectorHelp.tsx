import type { ReactNode } from "react";
import { Link } from "react-router";

interface GetModelSelectorHelpOptions {
	isModelCatalogLoading: boolean;
	hasModelOptions: boolean;
	hasConfiguredModels: boolean;
	hasUserFixableModelProviders: boolean;
}

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
			<Link
				to="/agents/settings/api-keys"
				className="underline transition-colors hover:text-content-primary"
			>
				Settings
			</Link>{" "}
			to enable models.
		</>
	);
};
