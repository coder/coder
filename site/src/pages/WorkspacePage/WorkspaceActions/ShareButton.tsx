import { workspaceSharingSettings } from "api/queries/organizations";
import type { Workspace } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { UsersIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { isGroup } from "modules/groups";
import { AddWorkspaceUserOrGroup } from "modules/workspaces/WorkspaceSharingForm/AddWorkspaceUserOrGroup";
import { useWorkspaceSharing } from "modules/workspaces/WorkspaceSharingForm/useWorkspaceSharing";
import { WorkspaceSharingForm } from "modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";
import type { FC } from "react";
import { useQuery } from "react-query";

interface ShareButtonProps {
	workspace: Workspace;
	canUpdatePermissions: boolean;
}

export const ShareButton: FC<ShareButtonProps> = ({
	workspace,
	canUpdatePermissions,
}) => {
	const { experiments } = useDashboard();
	const isWorkspaceSharingExperimentEnabled =
		experiments.includes("workspace-sharing");

	const workspaceSharingSettingsQuery = useQuery({
		...workspaceSharingSettings(workspace.organization_id),
		enabled: isWorkspaceSharingExperimentEnabled,
	});

	const sharing = useWorkspaceSharing(workspace);

	// Don't show the share button if:
	// 1. The experiment is not enabled, OR
	// 2. Workspace sharing is disabled for this organization.
	if (
		!isWorkspaceSharingExperimentEnabled ||
		workspaceSharingSettingsQuery.data?.sharing_disabled
	) {
		return null;
	}

	return (
		<Popover>
			<PopoverTrigger asChild>
				<TopbarButton data-testid="workspace-share-button">
					<UsersIcon />
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
