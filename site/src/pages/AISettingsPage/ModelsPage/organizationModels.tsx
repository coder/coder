import { isAxiosError } from "axios";
import { createContext, useContext } from "react";
import { useQueries, useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { chatModels } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import {
	canAccessOrganizationChatModelConfig,
	type OrganizationPermissions,
} from "#/modules/permissions/organizations";

export const modelOrganizationSearchParam = "org";

const isInaccessibleOrganizationError = (error: unknown): boolean =>
	isAxiosError(error) &&
	(error.response?.status === 403 || error.response?.status === 404);

export const useAccessibleModelOrganizations = (
	organizations: readonly Organization[],
) => {
	const queries = useQueries({
		queries: organizations.map((organization) => chatModels(organization.id)),
	});
	const permissionsQuery = useQuery(
		organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
	);
	const accessibleOrganizations = organizations.filter(
		(organization, index) =>
			(queries[index]?.data?.models?.length ?? 0) > 0 ||
			canAccessOrganizationChatModelConfig(
				permissionsQuery.data?.[organization.id],
			),
	);
	const requestError =
		queries.find(
			(query) => query.error && !isInaccessibleOrganizationError(query.error),
		)?.error ?? permissionsQuery.error;
	const hasData = accessibleOrganizations.length > 0;

	return {
		organizations: accessibleOrganizations,
		permissionsByOrganization: permissionsQuery.data,
		isLoading:
			queries.some((query) => query.isLoading) || permissionsQuery.isLoading,
		error: hasData ? null : (requestError ?? null),
		partialError: hasData ? (requestError ?? null) : null,
	};
};

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
