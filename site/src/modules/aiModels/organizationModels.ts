import { isAxiosError } from "axios";
import { useQueries, useQuery } from "react-query";
import { chatModels } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { canAccessOrganizationChatModelConfig } from "#/modules/permissions/organizations";

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
