import { updateWorkspaceACL } from "api/queries/workspaces";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import type { FC } from "react";
import { useMutation } from "react-query";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";

const localKirbyId = "1ce34e51-3135-4720-8bfc-eabce178eafb";
const devKirbyId = "7a4319a5-0dc1-41e1-95e4-f31e312b0ecc";

const WorkspaceSharingPage: FC = () => {
	const workspace = useWorkspaceSettings();
	const shareWithKirbyMutation = useMutation(updateWorkspaceACL(workspace.id));

	const onClick = () => {
		shareWithKirbyMutation.mutate({
			user_roles: {
				[localKirbyId]: "admin",
				[devKirbyId]: "admin",
			},
		});
	};

	return (
		<Button
			onClick={onClick}
			className=" bg-white hover:bg-pink-300 text-pink-800 hover:text-pink-950"
			size="lg"
		>
			<ExternalImage src="/kirby.gif" />
			Share with Kirby
		</Button>
	);
};

export default WorkspaceSharingPage;
