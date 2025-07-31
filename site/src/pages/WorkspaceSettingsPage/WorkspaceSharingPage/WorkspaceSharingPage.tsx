import { updateWorkspaceACL } from "api/queries/workspaces";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import type { FC } from "react";
import { useMutation } from "react-query";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";

const banditId = '75fb4089-d283-4697-aca2-d448944708ab';
const kirbyId = '7a4319a5-0dc1-41e1-95e4-f31e312b0ecc';

const WorkspaceSharingPage: FC = () => {
	const workspace = useWorkspaceSettings();
	const shareWithKirbyMutation = useMutation(updateWorkspaceACL(workspace.id));

	const onClick = () => {
		shareWithKirbyMutation.mutate({
			user_roles: { [banditId]: "admin", [kirbyId]: "admin" }
		});
	};

	return <Button onClick={onClick} className=" bg-white hover:bg-pink-300 text-pink-800 hover:text-pink-950" size="lg">
		<ExternalImage src="/kirby.gif" />
		Share with Kirby
	</Button>
};

export default WorkspaceSharingPage;
