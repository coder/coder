import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const WorkspaceHelpPopover: FC = () => {
	return (
		<InfoTooltip
			title="What is a workspace?"
			message={
				<>
					A workspace is your development environment in the cloud. It includes
					the infrastructure and tools you need to work on your project.
					<br />
					<Link size="sm" href={docs("/user-guides")}>
						Create Workspaces
					</Link>
					<br />
					<Link size="sm" href={docs("/user-guides/workspace-access")}>
						Connect with SSH
					</Link>
				</>
			}
		/>
	);
};
