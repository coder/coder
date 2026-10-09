import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	chatModelACL,
	chatModelACLAvailable,
	updateChatModelACL,
} from "#/api/queries/chats";
import { ACLPrincipalAutocomplete } from "../../components/ACLPrincipalAutocomplete";
import {
	selectedPrincipal,
	sharingDialogData,
} from "../../components/aclSharing";
import { ResourceSharingDialog } from "../../components/ResourceSharingDialog";

type ChatModelSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	modelId: string;
	modelName: string;
};

type OpenChatModelSharingDialogProps = Omit<
	ChatModelSharingDialogProps,
	"open"
>;

const OpenChatModelSharingDialog: React.FC<OpenChatModelSharingDialogProps> = ({
	onOpenChange,
	organizationId,
	modelId,
	modelName,
}) => {
	const queryClient = useQueryClient();
	const aclOptions = chatModelACL(organizationId, modelId);
	const aclQuery = useQuery({ ...aclOptions, refetchOnMount: "always" });
	const updateMutation = useMutation(updateChatModelACL(queryClient));
	const data = aclQuery.data ? sharingDialogData(aclQuery.data) : undefined;

	const close = () => {
		onOpenChange(false);
		queryClient.removeQueries({ queryKey: aclOptions.queryKey, exact: true });
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
						chatModelACLAvailable(organizationId, modelId, options)
					}
					excludedPrincipalIds={excludedPrincipalIds}
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

export const ChatModelSharingDialog: React.FC<ChatModelSharingDialogProps> = (
	props,
) => (props.open ? <OpenChatModelSharingDialog {...props} /> : null);
