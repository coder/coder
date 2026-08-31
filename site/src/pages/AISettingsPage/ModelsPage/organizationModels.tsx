import { createContext, useContext } from "react";
import { useSearchParams } from "react-router";
import type { Organization } from "#/api/typesGenerated";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";

export const modelOrganizationSearchParam = "org";

type ModelOrganizationSelection = {
	organization: Organization | undefined;
	requestedOrganizationDenied: boolean;
};

export const selectModelOrganization = (
	organizations: readonly Organization[],
	requestedName: string | null,
): ModelOrganizationSelection => {
	const requestedOrganization =
		requestedName === null
			? undefined
			: organizations.find(
					(organization) => organization.name === requestedName,
				);

	return {
		organization:
			requestedOrganization ??
			organizations.find((organization) => organization.is_default) ??
			organizations[0],
		requestedOrganizationDenied:
			requestedName !== null && requestedOrganization === undefined,
	};
};

type ModelQueryState = {
	data: unknown;
	error: unknown;
};

export const splitModelQueryErrors = (
	...queries: readonly ModelQueryState[]
): { loadError: unknown; refetchError: unknown } => {
	const loadError =
		queries.find((query) => query.data === undefined && query.error)?.error ??
		null;
	return {
		loadError,
		refetchError: loadError
			? null
			: (queries.find((query) => query.error)?.error ?? null),
	};
};

type OrganizationModelsContextValue = {
	organization: Organization;
	accessibleOrganizations: readonly Organization[];
	permissions: OrganizationPermissions | undefined;
	permissionsByOrganization?: Readonly<
		Record<string, OrganizationPermissions | undefined>
	>;
	requestedOrganizationDenied: boolean;
};

export const OrganizationModelsContext =
	createContext<OrganizationModelsContextValue | null>(null);

export const useOrganizationModels = (): OrganizationModelsContextValue => {
	const context = useContext(OrganizationModelsContext);
	if (!context) {
		throw new Error(
			"useOrganizationModels must be used within OrganizationModelsLayout",
		);
	}
	return context;
};

export const selectModelOrganizationPath = (
	pathname: string,
	organization: Organization,
	searchParams?: URLSearchParams,
): string => {
	const next = new URLSearchParams(searchParams);
	next.set(modelOrganizationSearchParam, organization.name);
	return `${pathname}?${next.toString()}`;
};

export const creatableModelOrganizations = (
	organizations: readonly Organization[],
	permissionsByOrganization?: Readonly<
		Record<string, OrganizationPermissions | undefined>
	>,
): readonly Organization[] =>
	organizations.filter(
		(organization) =>
			permissionsByOrganization?.[organization.id]?.createChatModelConfigs,
	);

const organizationModelSettingsPath = (
	organization: Organization,
	suffix: string,
	searchParams?: URLSearchParams,
): string => {
	const next = new URLSearchParams(searchParams);
	next.set(modelOrganizationSearchParam, organization.name);
	return `/ai/settings/models${suffix}?${next.toString()}`;
};

export const organizationAddModelPath = (
	organization: Organization,
	searchParams?: URLSearchParams,
): string => organizationModelSettingsPath(organization, "/add", searchParams);

export const organizationModelPath = (
	organization: Organization,
	modelId: string,
	searchParams?: URLSearchParams,
): string =>
	organizationModelSettingsPath(
		organization,
		`/${encodeURIComponent(modelId)}`,
		searchParams,
	);

export const useOrganizationModelsPath = (): string => {
	const { organization } = useOrganizationModels();
	const [searchParams] = useSearchParams();
	return organizationModelSettingsPath(organization, "", searchParams);
};
