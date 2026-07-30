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

const legacyModelsPath = "/ai/settings/models";

/**
 * Base path of the models pages inside the active organization context.
 * Falls back to the legacy redirect path when rendered without an
 * OrganizationModelsLayout (Storybook stories), which bounces to the
 * caller's preferred organization.
 */
export const useOrganizationModelsPath = (): string => {
	const context = useContext(OrganizationModelsContext);
	if (!context) {
		return legacyModelsPath;
	}
	return `/ai/settings/organizations/${context.organization.name}/models`;
};
