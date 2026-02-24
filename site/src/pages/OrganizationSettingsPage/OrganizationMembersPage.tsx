import { getErrorMessage } from "api/errors";
import { groupsByUserIdInOrganization } from "api/queries/groups";
import {
	addOrganizationMember,
	paginatedOrganizationMembers,
	removeOrganizationMember,
	updateOrganizationMemberRoles,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "hooks";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const OrganizationMembersPage: FC = () => {
	const queryClient = useQueryClient();
	const { user: me } = useAuthenticated();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const searchParamsResult = useSearchParams();

	const organizationRolesQuery = useQuery(organizationRoles(organizationName));
	const groupsByUserIdQuery = useQuery(
		groupsByUserIdInOrganization(organizationName),
	);

	const membersQuery = usePaginatedQuery(
		paginatedOrganizationMembers(organizationName, searchParamsResult[0]),
	);

	const members = membersQuery.data?.members.map(
		(member: OrganizationMemberWithUserData) => {
			const groups = groupsByUserIdQuery.data?.get(member.user_id) ?? [];
			return { ...member, groups };
		},
	);

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

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const title = (
		<title>
			{pageTitle("Members", organization.display_name || organization.name)}
		</title>
	);

	if (!organizationPermissions) {
		return (
			<>
				{title}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	return (
		<>
			{title}
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
				isAddingMember={addMemberMutation.isPending}
				isUpdatingMemberRoles={updateMemberRolesMutation.isPending}
				me={me}
				members={members}
				membersQuery={membersQuery}
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
				onConfirm={() => {
					if (memberToDelete) {
						const mutation = removeMemberMutation.mutateAsync(
							memberToDelete.user_id,
							{
								onSuccess: () => {
									membersQuery.refetch();
								},
							},
						);
						toast.promise(mutation, {
							loading: `Removing member "${memberToDelete.username}" from organization "${organization.display_name}"...`,
							success: `User "${memberToDelete.username}" removed from organization "${organization.display_name}" successfully.`,
							error: (error) =>
								getErrorMessage(
									error,
									`Failed to remove user "${memberToDelete.username}" from organization "${organization.display_name}".`,
								),
						});
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

						<p className="pb-5">Are you sure you want to remove this member?</p>
					</Stack>
				}
			/>
		</>
	);
};

export default OrganizationMembersPage;
