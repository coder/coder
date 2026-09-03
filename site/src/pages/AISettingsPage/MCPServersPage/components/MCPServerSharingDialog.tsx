import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	mcpServerConfigACL,
	updateMCPServerConfigACL,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	ResourceSharingDialog,
	type SharingDialogData,
	type SharingPrincipal,
	type SharingPrincipalSelection,
} from "../../components/ResourceSharingDialog";
import {
	MCPServerPrincipalAutocomplete,
	type MCPServerPrincipalAutocompleteValue,
} from "./MCPServerPrincipalAutocomplete";

type MCPServerSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	serverId: string;
	serverName: string;
};

type MCPServerPrincipal = Exclude<MCPServerPrincipalAutocompleteValue, null>;

const groupPrincipal = (group: TypesGen.Group): SharingPrincipal => ({
	id: group.id,
	name: group.display_name || group.name,
	subtitle: getGroupSubtitle(group),
	avatarUrl: group.avatar_url,
});

const userPrincipal = (user: TypesGen.MinimalUser): SharingPrincipal => ({
	id: user.id,
	name: user.username,
	subtitle: user.name || "User",
	avatarUrl: user.avatar_url,
});

const sharingDialogData = (
	acl: TypesGen.MCPServerConfigACL,
): SharingDialogData<TypesGen.MCPServerConfigRole> => ({
	acl: {
		user_roles: Object.fromEntries(
			acl.users.map((user) => [user.id, user.role]),
		),
		group_roles: Object.fromEntries(
			acl.groups.map((group) => [group.id, group.role]),
		),
	},
	principals: {
		users: Object.fromEntries(
			acl.users.map((user) => [user.id, userPrincipal(user)]),
		),
		groups: Object.fromEntries(
			acl.groups.map((group) => [group.id, groupPrincipal(group)]),
		),
	},
});

const selectedPrincipal = (
	option: MCPServerPrincipal,
): SharingPrincipalSelection =>
	isGroup(option)
		? { kind: "group", principal: groupPrincipal(option) }
		: { kind: "user", principal: userPrincipal(option) };

type OpenMCPServerSharingDialogProps = Omit<
	MCPServerSharingDialogProps,
	"open"
>;

const OpenMCPServerSharingDialog: FC<OpenMCPServerSharingDialogProps> = ({
	onOpenChange,
	organizationId,
	serverId,
	serverName,
}) => {
	const queryClient = useQueryClient();
	const aclOptions = mcpServerConfigACL(organizationId, serverId);
	const aclQuery = useQuery({ ...aclOptions, refetchOnMount: "always" });
	const updateMutation = useMutation(updateMCPServerConfigACL(queryClient));
	const data = aclQuery.data ? sharingDialogData(aclQuery.data) : undefined;

	const close = () => {
		onOpenChange(false);
		queryClient.removeQueries({ queryKey: aclOptions.queryKey, exact: true });
	};

	return (
		<ResourceSharingDialog
			title="Server permissions"
			description={
				<>Manage which organization members and groups can use {serverName}.</>
			}
			loadingLabel="Loading server permissions"
			emptyTitle="No members or groups have permission yet"
			tableLabel="Server permissions for members and groups"
			roleLabel="Read"
			confirmText="Save permissions"
			data={data}
			loadError={data ? null : aclQuery.error}
			refetchError={data ? aclQuery.error : null}
			saveError={updateMutation.error}
			isSaving={updateMutation.isPending}
			readRole="read"
			deletedRole=""
			renderAutocomplete={({ value, onChange, excludedPrincipalIds }) => (
				<MCPServerPrincipalAutocomplete
					organizationId={organizationId}
					value={value}
					onChange={onChange}
					serverId={serverId}
					excludedPrincipalIds={excludedPrincipalIds}
					className="w-full"
				/>
			)}
			getPrincipal={selectedPrincipal}
			onClose={close}
			onSave={(req) =>
				updateMutation.mutate(
					{ organization: organizationId, id: serverId, req },
					{
						onSuccess: () => {
							toast.success(`Permissions for "${serverName}" updated.`);
							close();
						},
					},
				)
			}
		/>
	);
};

export const MCPServerSharingDialog: FC<MCPServerSharingDialogProps> = (
	props,
) => (props.open ? <OpenMCPServerSharingDialog {...props} /> : null);
