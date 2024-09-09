import { groupsByUserId } from "api/queries/groups";
import {
	addOrganizationMember,
	organizationMembers,
	organizationPermissions,
	removeOrganizationMember,
	updateOrganizationMemberRoles,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const OrganizationMembersPage: FC = () => {
	const queryClient = useQueryClient();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { user: me } = useAuthenticated();

	const groupsByUserIdQuery = useQuery(groupsByUserId());

	const membersQuery = useQuery(organizationMembers(organizationName));
	const organizationRolesQuery = useQuery(organizationRoles(organizationName));

	const addMemberMutation = useMutation(
		addOrganizationMember(queryClient, organizationName),
	);
	const removeMemberMutation = useMutation(
		removeOrganizationMember(queryClient, organizationName),
	);
	const updateMemberRolesMutation = useMutation(
		updateOrganizationMemberRoles(queryClient, organizationName),
	);

	const { organizations } = useOrganizationSettings();
	const organization = organizations?.find((o) => o.name === organizationName);
	const permissionsQuery = useQuery(organizationPermissions(organization?.id));

	const permissions = permissionsQuery.data;
	if (!permissions) {
		return <Loader />;
	}

	return (
		<OrganizationMembersPageView
			allAvailableRoles={organizationRolesQuery.data}
			canEditMembers={permissions.editMembers}
			error={
				membersQuery.error ??
				addMemberMutation.error ??
				removeMemberMutation.error ??
				updateMemberRolesMutation.error
			}
			isAddingMember={addMemberMutation.isLoading}
			isUpdatingMemberRoles={updateMemberRolesMutation.isLoading}
			me={me}
			members={membersQuery.data}
			groupsByUserId={groupsByUserIdQuery.data}
			addMember={async (user: User) => {
				await addMemberMutation.mutateAsync(user.id);
				void membersQuery.refetch();
			}}
			removeMember={async (member: OrganizationMemberWithUserData) => {
				await removeMemberMutation.mutateAsync(member.user_id);
				void membersQuery.refetch();
			}}
			updateMemberRoles={async (
				member: OrganizationMemberWithUserData,
				newRoles: string[],
			) => {
				await updateMemberRolesMutation.mutateAsync({
					userId: member.user_id,
					roles: newRoles,
				});
			}}
		/>
	);
};

export default OrganizationMembersPage;
