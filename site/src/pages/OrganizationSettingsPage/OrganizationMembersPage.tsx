import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { groupsByUserIdInOrganization } from "#/api/queries/groups";
import {
	addOrganizationMember,
	paginatedOrganizationMembers,
	removeOrganizationMember,
	updateOrganizationMemberRoles,
} from "#/api/queries/organizations";
import { organizationRoles } from "#/api/queries/roles";
import type {
	OrganizationMemberWithUserData,
	User,
} from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { useFilter } from "#/components/Filter/Filter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { shouldShowAISeatColumn } from "#/modules/dashboard/entitlements";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { RoleSelectorDialog } from "#/modules/roles/RoleSelectorDialog";
import { pageTitle } from "#/utils/page";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const OrganizationMembersPage: FC = () => {
	const queryClient = useQueryClient();
	const { user: me } = useAuthenticated();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { entitlements } = useDashboard();
	const searchParamsResult = useSearchParams();
	const showAISeatColumn = shouldShowAISeatColumn(entitlements);

	const organizationRolesQuery = useQuery(organizationRoles(organizationName));
	const groupsByUserIdQuery = useQuery(
		groupsByUserIdInOrganization(organizationName),
	);

	const membersQuery = usePaginatedQuery(
		paginatedOrganizationMembers(organizationName, searchParamsResult[0]),
	);
	const filterProps = useFilter({
		searchParams: searchParamsResult[0],
		onSearchParamsChange: searchParamsResult[1],
		onUpdate: membersQuery.goToFirstPage,
	});

	const members = membersQuery.data?.members.map(
		(member: OrganizationMemberWithUserData) => {
			const groups = groupsByUserIdQuery.data?.get(member.user_id) ?? [];
			return { ...member, groups };
		},
	);

	const addMemberMutation = useMutation(
		addOrganizationMember(queryClient, organizationName),
	);

	const [memberToEditRoles, setMemberToEditRoles] =
		useState<OrganizationMemberWithUserData>();
	const updateMemberRolesMutation = useMutation(
		updateOrganizationMemberRoles(queryClient, organizationName),
	);

	const [memberToRemove, setMemberToRemove] =
		useState<OrganizationMemberWithUserData>();
	const removeMemberMutation = useMutation(
		removeOrganizationMember(queryClient, organizationName),
	);

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
				error={
					membersQuery.error ??
					organizationRolesQuery.error ??
					groupsByUserIdQuery.error ??
					addMemberMutation.error ??
					removeMemberMutation.error ??
					updateMemberRolesMutation.error
				}
				filterProps={{ filter: filterProps }}
				organizationName={organizationName}
				membersQuery={membersQuery}
				members={members}
				showAISeatColumn={showAISeatColumn}
				addMembers={async (users: User[]) => {
					// TODO: Replace with a batch endpoint (POST /organizations/{org}/members)
					// to add all users in a single request instead of N individual calls.
					// See branch jakehwll/devex-112-organizations-batch-endpoint.
					await Promise.all(
						users.map((user) => addMemberMutation.mutateAsync(user.id)),
					);
					void membersQuery.refetch();
				}}
				onEditMemberRoles={setMemberToEditRoles}
				isUpdatingMemberRoles={updateMemberRolesMutation.isPending}
				removeMember={setMemberToRemove}
				me={me.id}
				canEditMembers={organizationPermissions.editMembers}
				canViewMembers={organizationPermissions.viewMembers}
				canViewActivity={entitlements.features.audit_log.enabled}
			/>

			<RoleSelectorDialog
				key={memberToEditRoles?.username}
				user={memberToEditRoles}
				availableRoles={organizationRolesQuery.data}
				onCancel={() => setMemberToEditRoles(undefined)}
				onUpdateRoles={async (roles) => {
					try {
						await updateMemberRolesMutation.mutateAsync({
							userId: memberToEditRoles!.user_id,
							roles,
						});
						toast.success(
							`${memberToEditRoles!.username}'s roles have been updated.`,
						);
						setMemberToEditRoles(undefined);
					} catch (e) {
						toast.error(getErrorMessage(e, "Error updating member roles."), {
							description: getErrorDetail(e),
						});
					}
				}}
				isUpdatingRoles={updateMemberRolesMutation.isPending}
			/>

			<ConfirmDialog
				type="delete"
				open={memberToRemove !== undefined}
				onClose={() => setMemberToRemove(undefined)}
				title="Remove member"
				confirmText="Remove"
				onConfirm={() => {
					if (memberToRemove) {
						const mutation = removeMemberMutation.mutateAsync(
							memberToRemove.user_id,
							{
								onSuccess: () => {
									membersQuery.refetch();
								},
							},
						);
						toast.promise(mutation, {
							loading: `Removing "${memberToRemove.username}" from "${organization.display_name}"...`,
							success: `"${memberToRemove.username}" has been removed from "${organization.display_name}".`,
							error: (error) =>
								getErrorMessage(
									error,
									`Failed to remove "${memberToRemove.username}" from "${organization.display_name}".`,
								),
						});
						setMemberToRemove(undefined);
					}
				}}
				description={
					<div className="flex flex-col gap-4">
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
					</div>
				}
			/>
		</>
	);
};

export default OrganizationMembersPage;
