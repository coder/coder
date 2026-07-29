import { createContext, useContext } from "react";
import type { Organization } from "#/api/typesGenerated";

type OrganizationModelsContextValue = {
	organization: Organization;
	organizations: readonly Organization[];
};

/**
 * The organization whose chat model configs the current /ai/settings
 * models pages manage. Resolved from the :organization route param by
 * OrganizationModelsLayout.
 */
export const OrganizationModelsContext =
	createContext<OrganizationModelsContextValue | null>(null);

export const useOrganizationModels = (): OrganizationModelsContextValue => {
	const context = useContext(OrganizationModelsContext);
	if (!context) {
		throw new Error(
			"useOrganizationModels only can be used inside of OrganizationModelsLayout",
		);
	}
	return context;
};
