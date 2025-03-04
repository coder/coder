import type { Interpolation, Theme } from "@emotion/react";
import { getErrorMessage } from "api/errors";
import { groupsByUserIdInOrganization } from "api/queries/groups";
import {
	addOrganizationMember,
	organizationMembers,
	paginatedMembers,
	removeOrganizationMember,
	updateOrganizationMemberRoles,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const OrganizationMembersPage: FC = () => {
	const queryClient = useQueryClient();
	const searchParamsResult = useSearchParams();
	const { user: me } = useAuthenticated();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [searchParams] = searchParamsResult;

	const membersQuery = usePaginatedQuery(
		paginatedMembers(organizationName, searchParamsResult[0]),
	);
	const organizationRolesQuery = useQuery(organizationRoles(organizationName));
	const groupsByUserIdQuery = useQuery(
		groupsByUserIdInOrganization(organizationName),
	);

	const members = membersQuery.data?.members.map((member) => {
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

	const [memberToDelete, setMemberToDelete] =
		useState<OrganizationMemberWithUserData>();

	if (!organization || !organizationPermissions) {
		return <EmptyState message="Organization not found" />;
	}

	const helmet = (
		<Helmet>
			<title>
				{pageTitle("Members", organization.display_name || organization.name)}
			</title>
		</Helmet>
	);

	return (
		<>
			{helmet}
			<OrganizationMembersPageView
				allAvailableRoles={organizationRolesQuery.data}
				canEditMembers={organizationPermissions.editMembers}
				canViewMembers={organizationPermissions.viewMembers}
				error={
					membersQuery.error ??
					organizationRolesQuery.error ??
					groupsByUserIdQuery.error ??
					addMemberMutation.error ??
					removeMemberMutation.error ??
					updateMemberRolesMutation.error
				}
				isAddingMember={addMemberMutation.isLoading}
				isUpdatingMemberRoles={updateMemberRolesMutation.isLoading}
				me={me}
				members={members}
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
