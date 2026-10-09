import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	mcpServerConfigACL,
	mcpServerConfigACLAvailable,
	updateMCPServerConfigACL,
} from "#/api/queries/chats";
import { ACLPrincipalAutocomplete } from "../../components/ACLPrincipalAutocomplete";
import {
	selectedPrincipal,
	sharingDialogData,
} from "../../components/aclSharing";
import { ResourceSharingDialog } from "../../components/ResourceSharingDialog";

type MCPServerSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	serverId: string;
	serverName: string;
};

type OpenMCPServerSharingDialogProps = Omit<
	MCPServerSharingDialogProps,
	"open"
>;

const OpenMCPServerSharingDialog: React.FC<OpenMCPServerSharingDialogProps> = ({
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
				<ACLPrincipalAutocomplete
					value={value}
					onChange={onChange}
					availableQuery={(options) =>
						mcpServerConfigACLAvailable(organizationId, serverId, options)
					}
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

export const MCPServerSharingDialog: React.FC<MCPServerSharingDialogProps> = (
	props,
) => (props.open ? <OpenMCPServerSharingDialog {...props} /> : null);
