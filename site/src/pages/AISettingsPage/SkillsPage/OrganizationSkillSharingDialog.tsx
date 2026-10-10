import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	organizationSkillACL,
	organizationSkillACLAvailable,
	updateOrganizationSkillACL,
} from "#/api/queries/skills";
import { ACLPrincipalAutocomplete } from "../components/ACLPrincipalAutocomplete";
import { selectedPrincipal, sharingDialogData } from "../components/aclSharing";
import { ResourceSharingDialog } from "../components/ResourceSharingDialog";

type OrganizationSkillSharingDialogProps = {
	organizationId: string;
	skillName: string;
	onClose: () => void;
	onCloseAutoFocus?: (event: Event) => void;
};

export const OrganizationSkillSharingDialog: React.FC<
	OrganizationSkillSharingDialogProps
> = ({ organizationId, skillName, onClose, onCloseAutoFocus }) => {
	const queryClient = useQueryClient();
	const aclOptions = organizationSkillACL(organizationId, skillName);
	const aclQuery = useQuery({ ...aclOptions, refetchOnMount: "always" });
	const updateMutation = useMutation(updateOrganizationSkillACL(queryClient));
	const data = aclQuery.data ? sharingDialogData(aclQuery.data) : undefined;

	const close = () => {
		onClose();
		queryClient.removeQueries({ queryKey: aclOptions.queryKey, exact: true });
	};

	return (
		<ResourceSharingDialog
			title="Skill permissions"
			description={
				<>
					Manage which organization members and groups can use {skillName}. Site
					owners, organization admins, and auditors can use it whenever it is
					enabled.
				</>
			}
			loadingLabel="Loading skill permissions"
			emptyTitle="No members or groups have permission yet"
			tableLabel="Skill permissions for members and groups"
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
					availableQueryOptions={(options) =>
						organizationSkillACLAvailable(organizationId, skillName, options)
					}
					excludedPrincipalIds={excludedPrincipalIds}
				/>
			)}
			getPrincipal={selectedPrincipal}
			onClose={close}
			onCloseAutoFocus={onCloseAutoFocus}
			onSave={(req) =>
				updateMutation.mutate(
					{ organizationId, name: skillName, req },
					{
						onSuccess: () => {
							toast.success(`Permissions for "${skillName}" updated.`);
							close();
						},
					},
				)
			}
		/>
	);
};
