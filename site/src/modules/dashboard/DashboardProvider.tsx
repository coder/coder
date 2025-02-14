import { appearance } from "api/queries/appearance";
import { entitlements } from "api/queries/entitlements";
import { experiments } from "api/queries/experiments";
import {
	anyOrganizationPermissions,
	organizations,
} from "api/queries/organizations";
import type {
	AppearanceConfig,
	Entitlements,
	Experiments,
	Organization,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { canViewAnyOrganization } from "modules/management/organizationPermissions";
import { type FC, type PropsWithChildren, createContext } from "react";
import { useQuery } from "react-query";
import { selectFeatureVisibility } from "./entitlements";

export interface DashboardValue {
	entitlements: Entitlements;
	experiments: Experiments;
	appearance: AppearanceConfig;
	organizations: readonly Organization[];
	showOrganizations: boolean;
	canViewOrganizationSettings: boolean;
}

export const DashboardContext = createContext<DashboardValue | undefined>(
	undefined,
);

export const DashboardProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const entitlementsQuery = useQuery(entitlements(metadata.entitlements));
	const experimentsQuery = useQuery(experiments(metadata.experiments));
	const appearanceQuery = useQuery(appearance(metadata.appearance));
	const organizationsQuery = useQuery(organizations());
	const anyOrganizationPermissionsQuery = useQuery(
		anyOrganizationPermissions(),
	);

	const error =
		entitlementsQuery.error ||
		appearanceQuery.error ||
		experimentsQuery.error ||
		organizationsQuery.error ||
		anyOrganizationPermissionsQuery.error;

	if (error) {
		return <ErrorAlert error={error} />;
	}

	const isLoading =
		!entitlementsQuery.data ||
		!appearanceQuery.data ||
		!experimentsQuery.data ||
		!organizationsQuery.data ||
		!anyOrganizationPermissionsQuery.data;

	if (isLoading) {
		return <Loader fullscreen />;
	}

	const hasMultipleOrganizations = organizationsQuery.data.length > 1;
	const organizationsEnabled = selectFeatureVisibility(
		entitlementsQuery.data,
	).multiple_organizations;
	const showOrganizations = hasMultipleOrganizations || organizationsEnabled;

	return (
		<DashboardContext.Provider
			value={{
				entitlements: entitlementsQuery.data,
				experiments: experimentsQuery.data,
				appearance: appearanceQuery.data,
				organizations: organizationsQuery.data,
				showOrganizations,
				canViewOrganizationSettings:
					showOrganizations &&
					canViewAnyOrganization(anyOrganizationPermissionsQuery.data),
			}}
		>
			{children}
		</DashboardContext.Provider>
	);
};
