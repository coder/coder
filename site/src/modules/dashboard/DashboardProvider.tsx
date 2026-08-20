import { createContext, type FC, type PropsWithChildren } from "react";
import { useQuery } from "react-query";
import { appearance } from "#/api/queries/appearance";
import { buildInfo } from "#/api/queries/buildInfo";
import { entitlements } from "#/api/queries/entitlements";
import { experiments } from "#/api/queries/experiments";
import { organizations } from "#/api/queries/organizations";
import type {
	AppearanceConfig,
	BuildInfoResponse,
	Entitlements,
	Experiment,
	Organization,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { canViewAnyOrganization } from "#/modules/permissions";
import { selectFeatureVisibility } from "./entitlements";

export interface DashboardValue {
	entitlements: Entitlements;
	experiments: Experiment[];
	appearance: AppearanceConfig;
	buildInfo: BuildInfoResponse;
	organizations: readonly Organization[];
	showOrganizations: boolean;
	canViewOrganizationSettings: boolean;
}

export const DashboardContext = createContext<DashboardValue | undefined>(
	undefined,
);

export const DashboardProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const { permissions } = useAuthenticated();
	const entitlementsQuery = useQuery({
		...entitlements(metadata.entitlements),
		// cachedQuery disables all refetches when embedded metadata seeds the
		// cache, so poll to keep usage-driven warnings current.
		refetchInterval: 5 * 60 * 1000,
		refetchIntervalInBackground: false,
	});
	const experimentsQuery = useQuery(experiments(metadata.experiments));
	const appearanceQuery = useQuery(appearance(metadata.appearance));
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const organizationsQuery = useQuery(organizations(metadata.organizations));

	// Entitlements poll, so a transient refetch error must not replace a
	// rendered dashboard; an error is fatal only without cached data.
	const error =
		(!entitlementsQuery.data && entitlementsQuery.error) ||
		(!appearanceQuery.data && appearanceQuery.error) ||
		(!experimentsQuery.data && experimentsQuery.error) ||
		(!buildInfoQuery.data && buildInfoQuery.error) ||
		(!organizationsQuery.data && organizationsQuery.error);

	if (error) {
		return <ErrorAlert error={error} />;
	}

	const isLoading =
		!entitlementsQuery.data ||
		!appearanceQuery.data ||
		!experimentsQuery.data ||
		!buildInfoQuery.data ||
		!organizationsQuery.data;

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
				buildInfo: buildInfoQuery.data,
				organizations: organizationsQuery.data,
				showOrganizations,
				canViewOrganizationSettings:
					showOrganizations && canViewAnyOrganization(permissions),
			}}
		>
			{children}
		</DashboardContext.Provider>
	);
};
