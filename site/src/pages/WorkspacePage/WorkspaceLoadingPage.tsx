import type { Workspace } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { ProvisionerStatusAlert } from "modules/provisioners/ProvisionerStatusAlert";
import type { FC } from "react";

interface WorkspaceLoadingPageProps {
	workspace?: Workspace;
}

export const WorkspaceLoadingPage: FC<WorkspaceLoadingPageProps> = ({
	workspace,
}) => {
	const shouldShowProvisionerStatusAlert =
		workspace && workspace.latest_build.status === "pending";
	return (
		<>
			{shouldShowProvisionerStatusAlert && (
				<ProvisionerStatusAlert
					matchingProvisioners={
						workspace?.latest_build.matched_provisioners?.count
					}
					availableProvisioners={
						workspace?.latest_build.matched_provisioners?.available
					}
					tags={workspace?.latest_build.job.tags || {}}
				/>
			)}
			<Loader />
		</>
	);
};
