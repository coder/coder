import { useQuery } from "react-query";
import { checkAuthorization } from "#/api/queries/authCheck";
import type { WorkspacePermissions } from "#/modules/workspaces/permissions";
import { workspaceChecks } from "#/modules/workspaces/permissions";
import { useWorkspaceSharing } from "#/modules/workspaces/WorkspaceSharingForm/useWorkspaceSharing";
import { pageTitle } from "#/utils/page";
import { useWorkspaceSettings } from "../useWorkspaceSettings";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const WorkspaceSharingPage: React.FC = () => {
	const { workspace } = useWorkspaceSettings();
	const sharing = useWorkspaceSharing(workspace);

	const checks = workspaceChecks(workspace);
	const permissionsQuery = useQuery({
		...checkAuthorization<WorkspacePermissions>({ checks }),
	});
	const permissions = permissionsQuery.data;
	const canShareWorkspace = Boolean(permissions?.shareWorkspace);

	const error =
		sharing.error ?? permissionsQuery.error ?? sharing.mutationError;

	return (
		<>
			<title>{pageTitle(workspace.name, "Sharing")}</title>

			<WorkspaceSharingPageView
				workspace={workspace}
				workspaceACL={sharing.workspaceACL}
				canShareWorkspace={canShareWorkspace}
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
