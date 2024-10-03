import type { Interpolation, Theme } from "@emotion/react";
import { getErrorMessage } from "api/errors";
import { groupsByUserIdInOrganization } from "api/queries/groups";
import {
	addOrganizationMember,
	organizationMembers,
	organizationPermissions,
	removeOrganizationMember,
	updateOrganizationMemberRoles,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const OrganizationMembersPage: FC = () => {
	const queryClient = useQueryClient();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { user: me } = useAuthenticated();

	const groupsByUserIdQuery = useQuery(
		groupsByUserIdInOrganization(organizationName),
	);

	const membersQuery = useQuery(organizationMembers(organizationName));
	const organizationRolesQuery = useQuery(organizationRoles(organizationName));

	const members = membersQuery.data?.map((member) => {
		const groups = groupsByUserIdQuery.data?.get(member.user_id) ?? [];
		return { ...member, groups };
	});

	const addMemberMutation = useMutation(
		addOrganizationMember(queryClient, organizationName),
	);
	const removeMemberMutation = useMutation(
		removeOrganizationMember(queryClient, organizationName),
	);
	const updateMemberRolesMutation = useMutation(
		updateOrganizationMemberRoles(queryClient, organizationName),
	);

	const { organizations } = useManagementSettings();
	const organization = organizations?.find((o) => o.name === organizationName);
	const permissionsQuery = useQuery(organizationPermissions(organization?.id));

	const [memberToDelete, setMemberToDelete] =
		useState<OrganizationMemberWithUserData>();

	const permissions = permissionsQuery.data;
	if (!permissions) {
		return <Loader />;
	}

	return (
		<>
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
				members={members}
				groupsByUserId={groupsByUserIdQuery.data}
				addMember={async (user: User) => {
					await addMemberMutation.mutateAsync(user.id);
					void membersQuery.refetch();
				}}
				removeMember={setMemberToDelete}
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

			<ConfirmDialog
				type="delete"
				open={memberToDelete !== undefined}
				onClose={() => setMemberToDelete(undefined)}
				title="Remove member"
				confirmText="Remove"
				onConfirm={async () => {
					try {
						if (memberToDelete) {
							await removeMemberMutation.mutateAsync(memberToDelete?.user_id);
						}
						setMemberToDelete(undefined);
						await membersQuery.refetch();
						displaySuccess("User removed from organization successfully!");
					} catch (error) {
						setMemberToDelete(undefined);
						displayError(
							getErrorMessage(error, "Failed to remove user from organization"),
						);
					} finally {
						setMemberToDelete(undefined);
					}
				}}
				description={
					<Stack>
						<p>
							Removing this member will:
							<ul>
								<li>Remove the member from all groups in this organization</li>
								<li>Remove all user role assignments</li>
								<li>
									Orphan all the member's workspaces associated with this
									organization
								</li>
							</ul>
						</p>

						<p css={styles.test}>
							Are you sure you want to remove this member?
						</p>
					</Stack>
				}
			/>
		</>
	);
};

const styles = {
	test: {
		paddingBottom: 20,
	},
} satisfies Record<string, Interpolation<Theme>>;

export default OrganizationMembersPage;
