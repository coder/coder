import { useWorkspaceSharing } from "#/modules/workspaces/WorkspaceSharingForm/useWorkspaceSharing";
import { pageTitle } from "#/utils/page";
import { useWorkspaceSettings } from "../useWorkspaceSettings";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const WorkspaceSharingPage: React.FC = () => {
	const { workspace, permissions } = useWorkspaceSettings();
	const sharing = useWorkspaceSharing(workspace);

	const error = sharing.error ?? sharing.mutationError;

	return (
		<>
			<title>{pageTitle(workspace.name, "Sharing")}</title>

			<WorkspaceSharingPageView
				workspace={workspace}
				workspaceACL={sharing.workspaceACL}
				canShareWorkspace={permissions.shareWorkspace}
				error={error}
				onAddUser={sharing.addUser}
				isAddingUser={sharing.isAddingUser}
				onUpdateUser={sharing.updateUser}
				updatingUserId={sharing.updatingUserId}
				onRemoveUser={sharing.removeUser}
				onAddGroup={sharing.addGroup}
				isAddingGroup={sharing.isAddingGroup}
				onUpdateGroup={sharing.updateGroup}
				updatingGroupId={sharing.updatingGroupId}
				onRemoveGroup={sharing.removeGroup}
				hasRemovedMember={sharing.hasRemovedMember}
			/>
		</>
	);
};

export default WorkspaceSharingPage;
