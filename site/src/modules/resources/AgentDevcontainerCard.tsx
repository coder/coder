import Link from "@mui/material/Link";
import type { WorkspaceAgentDevcontainer } from "api/typesGenerated";
import type { FC } from "react";
import { AgentButton } from "./AgentButton";
import { AgentDevcontainerSSHButton } from "./SSHButton/SSHButton";

type AgentDevcontainerCardProps = {
	container: WorkspaceAgentDevcontainer;
	workspace: string;
};

export const AgentDevcontainerCard: FC<AgentDevcontainerCardProps> = ({
	container,
	workspace,
}) => {
	return (
		<section
			className="border border-border border-dashed rounded p-6 "
			key={container.id}
		>
			<header className="flex justify-between">
				<h3 className="m-0 text-xs font-medium text-content-secondary">
					{container.name}
				</h3>

				<AgentDevcontainerSSHButton
					workspace={workspace}
					container={container.name}
				/>
			</header>

			<h4 className="m-0 text-xl font-semibold">Forwarded ports</h4>

			<div className="flex gap-4 flex-wrap mt-4">
				{container.ports.map((port) => {
					return (
						<Link
							key={port.port}
							color="inherit"
							component={AgentButton}
							underline="none"
						>
							{port.process_name ||
								`${port.port}/${port.network.toUpperCase()}`}
						</Link>
					);
				})}
			</div>
		</section>
	);
};
