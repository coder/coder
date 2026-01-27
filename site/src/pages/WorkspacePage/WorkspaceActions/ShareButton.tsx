import type { Workspace } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Share2Icon } from "lucide-react";
import { isGroup } from "modules/groups";
import { AddWorkspaceUserOrGroup } from "modules/workspaces/WorkspaceSharingForm/AddWorkspaceUserOrGroup";
import { useWorkspaceSharing } from "modules/workspaces/WorkspaceSharingForm/useWorkspaceSharing";
import { WorkspaceSharingForm } from "modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";
import type { FC } from "react";

interface ShareButtonProps {
	workspace: Workspace;
	canUpdatePermissions: boolean;
}

export const ShareButton: FC<ShareButtonProps> = ({
	workspace,
	canUpdatePermissions,
}) => {
	const sharing = useWorkspaceSharing(workspace);

	return (
		<Popover>
			<PopoverTrigger asChild>
				<TopbarButton data-testid="workspace-share-button">
					<Share2Icon />
					Share
				</TopbarButton>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-[580px] p-4">
				<WorkspaceSharingForm
					workspaceACL={sharing.workspaceACL}
					canUpdatePermissions={canUpdatePermissions}
					error={sharing.error ?? sharing.mutationError}
					updatingUserId={sharing.updatingUserId}
					onUpdateUser={sharing.updateUser}
					onRemoveUser={sharing.removeUser}
					updatingGroupId={sharing.updatingGroupId}
					onUpdateGroup={sharing.updateGroup}
					onRemoveGroup={sharing.removeGroup}
					showRestartWarning={sharing.hasRemovedMember}
					isCompact
					addMemberForm={
						<AddWorkspaceUserOrGroup
							organizationID={workspace.organization_id}
							workspaceACL={sharing.workspaceACL}
							isLoading={sharing.isAddingUser || sharing.isAddingGroup}
							onSubmit={(value, role, resetAutocomplete) =>
								isGroup(value)
									? sharing.addGroup(value, role, resetAutocomplete)
									: sharing.addUser(value, role, resetAutocomplete)
							}
						/>
					}
				/>
			</PopoverContent>
		</Popover>
	);
};
