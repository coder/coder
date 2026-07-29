import { useContext } from "react";
import { OrganizationModelsContext } from "./organizationModels";

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
