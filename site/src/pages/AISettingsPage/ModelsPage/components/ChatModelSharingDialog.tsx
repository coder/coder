import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { chatModelACL, updateChatModelACL } from "#/api/queries/chats";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";
import {
	ResourceSharingDialog,
	type SharingDialogData,
	type SharingPrincipal,
	type SharingPrincipalSelection,
} from "../../components/ResourceSharingDialog";

type ChatModelSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	modelId: string;
	modelName: string;
};

type ChatModelPrincipal = Exclude<UserOrGroupAutocompleteValue, null>;

const groupPrincipal = (group: TypesGen.Group): SharingPrincipal => ({
	id: group.id,
	name: group.display_name || group.name,
	subtitle: getGroupSubtitle(group),
	avatarUrl: group.avatar_url,
});

const memberPrincipal = (
	member: TypesGen.OrganizationMemberWithUserData,
): SharingPrincipal => ({
	id: member.user_id,
	name: member.username,
	subtitle: member.name || member.email || "User",
	avatarUrl: member.avatar_url,
});

const sharingDialogData = (
	acl: TypesGen.ChatModelACL,
	members: readonly TypesGen.OrganizationMemberWithUserData[],
	groups: readonly TypesGen.Group[],
): SharingDialogData<TypesGen.ChatRole> => ({
	acl: {
		user_roles: { ...acl.user_roles },
		group_roles: { ...acl.group_roles },
	},
	principals: {
		users: Object.fromEntries(
			Object.keys(acl.user_roles).map((userId) => {
				const member = members.find((item) => item.user_id === userId);
				return [
					userId,
					member
						? memberPrincipal(member)
						: { id: userId, name: userId, subtitle: "User" },
				];
			}),
		),
		groups: Object.fromEntries(
			Object.keys(acl.group_roles).map((groupId) => {
				const group = groups.find((item) => item.id === groupId);
				return [
					groupId,
					group
						? groupPrincipal(group)
						: { id: groupId, name: groupId, subtitle: "Group" },
				];
			}),
		),
	},
});

const selectedPrincipal = (
	option: ChatModelPrincipal,
): SharingPrincipalSelection =>
	isGroup(option)
		? { kind: "group", principal: groupPrincipal(option) }
		: { kind: "user", principal: memberPrincipal(option) };

type OpenChatModelSharingDialogProps = Omit<
	ChatModelSharingDialogProps,
	"open"
>;

const OpenChatModelSharingDialog: FC<OpenChatModelSharingDialogProps> = ({
	onOpenChange,
	organizationId,
	modelId,
	modelName,
}) => {
	const queryClient = useQueryClient();
	const aclOptions = chatModelACL(organizationId, modelId);
	const membersOptions = organizationMembers(organizationId, { limit: 0 });
	const groupsOptions = groupsByOrganization(organizationId);
	const aclQuery = useQuery({ ...aclOptions, refetchOnMount: "always" });
	const membersQuery = useQuery({
		...membersOptions,
		refetchOnMount: "always",
	});
	const groupsQuery = useQuery({ ...groupsOptions, refetchOnMount: "always" });
	const updateMutation = useMutation(updateChatModelACL(queryClient));

	const members = membersQuery.data?.members;
	const groups = groupsQuery.data;
	const data =
		aclQuery.data && members && groups
			? sharingDialogData(aclQuery.data, members, groups)
			: undefined;
	const loadError = data
		? null
		: (aclQuery.error ??
			(members === undefined ? membersQuery.error : null) ??
			(groups === undefined ? groupsQuery.error : null));
	const refetchError = data
		? (aclQuery.error ?? membersQuery.error ?? groupsQuery.error)
		: null;

	const close = () => {
		onOpenChange(false);
		queryClient.removeQueries({ queryKey: aclOptions.queryKey, exact: true });
		queryClient.removeQueries({
			queryKey: membersOptions.queryKey,
			exact: true,
		});
		queryClient.removeQueries({
			queryKey: groupsOptions.queryKey,
			exact: true,
		});
	};

	return (
		<ResourceSharingDialog
			title="Model permissions"
			description={
				<>Manage which organization members and groups can use {modelName}.</>
			}
			loadingLabel="Loading model permissions"
			emptyTitle="No members or groups have permission yet"
			tableLabel="Model permissions for members and groups"
			roleLabel="Use"
			confirmText="Save permissions"
			data={data}
			loadError={loadError}
			refetchError={refetchError}
			saveError={updateMutation.error}
			isSaving={updateMutation.isPending}
			readRole="read"
			deletedRole=""
			renderAutocomplete={({ value, onChange, excludedPrincipalIds }) => (
				<UserOrGroupAutocomplete
					organizationId={organizationId}
					value={value}
					onChange={onChange}
					exclude={excludedPrincipalIds.map((id) => ({ id }))}
					className="w-full"
				/>
			)}
			getPrincipal={selectedPrincipal}
			onClose={close}
			onSave={(req) =>
				updateMutation.mutate(
					{ organizationId, modelId, req },
					{
						onSuccess: () => {
							toast.success(`Permissions for "${modelName}" updated.`);
							close();
						},
					},
				)
			}
		/>
	);
};

export const ChatModelSharingDialog: FC<ChatModelSharingDialogProps> = (
	props,
) => (props.open ? <OpenChatModelSharingDialog {...props} /> : null);
