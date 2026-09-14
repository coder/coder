import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createOrganizationRole,
	organizationRoles,
	updateOrganizationRole,
} from "#/api/queries/roles";
import type { CustomRoleRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { CreateEditRolePageView } from "./CreateEditRolePageView";

const CreateEditRolePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { organization: organizationName, roleName } = useParams();
	const { organizationPermissions } = useOrganizationSettings();
	const rolesQuery = useQuery({
		...organizationRoles(organizationName ?? ""),
		enabled: Boolean(organizationName),
	});
	const createOrganizationRoleMutation = useMutation(
		createOrganizationRole(queryClient, organizationName ?? ""),
	);
	const updateOrganizationRoleMutation = useMutation(
		updateOrganizationRole(queryClient, organizationName ?? ""),
	);

	if (!organizationName) {
		return <EmptyState message="Organization not found" />;
	}

	const rolesHref = `/organizations/${organizationName}/roles`;

	if (rolesQuery.isLoading) {
		return <Loader />;
	}

	if (rolesQuery.error) {
		return <ErrorAlert error={rolesQuery.error} />;
	}

	if (!organizationPermissions) {
		return <ErrorAlert error="Failed to load organization permissions" />;
	}

	const role = roleName
		? rolesQuery.data?.find((candidate) => candidate.name === roleName)
		: undefined;

	if (roleName && !role) {
		return (
			<EmptyState
				message="Role not found"
				cta={
					<Button variant="outline" asChild>
						<Link to={rolesHref}>Back to roles</Link>
					</Button>
				}
			/>
		);
	}

	const isEditing = role !== undefined;
	const saveRole = isEditing
		? updateOrganizationRoleMutation
		: createOrganizationRoleMutation;

	const handleSubmit = (data: CustomRoleRequest) => {
		const mutation = saveRole.mutateAsync(data, {
			onSuccess: () => {
				navigate(rolesHref);
			},
		});
		toast.promise(mutation, {
			loading: `${isEditing ? "Updating" : "Creating"} custom role "${data.name}"...`,
			success: `Custom role "${data.name}" ${isEditing ? "updated" : "created"} successfully.`,
			error: (error) => ({
				message: getErrorMessage(
					error,
					`Failed to ${isEditing ? "update" : "create"} custom role "${data.name}".`,
				),
				description: getErrorDetail(error),
			}),
		});
	};

	return (
		<RequirePermission
			isFeatureVisible={
				isEditing
					? organizationPermissions.updateOrgRoles
					: organizationPermissions.createOrgRoles
			}
		>
			<title>
				{pageTitle(
					isEditing ? "Edit Custom Role" : "New Custom Role",
					isEditing ? role.display_name || role.name : undefined,
				)}
			</title>

			<CreateEditRolePageView
				role={role}
				onSubmit={handleSubmit}
				error={saveRole.error}
				isLoading={saveRole.isPending}
				organizationName={organizationName}
			/>
		</RequirePermission>
	);
};

export default CreateEditRolePage;
