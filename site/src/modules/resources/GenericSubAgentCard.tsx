import { BotIcon } from "lucide-react";
import type { FC } from "react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { AgentApps, organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentStatus } from "./AgentStatus";

type GenericSubAgentCardProps = {
	subAgent: WorkspaceAgent;
	workspace: Workspace;
};

/**
 * Renders a child agent that is not backed by a dev container, such as an
 * agent created by a template for isolated execution. It shows the child
 * name, its connection status and its apps so they remain reachable from the
 * parent agent row.
 */
export const GenericSubAgentCard: FC<GenericSubAgentCardProps> = ({
	subAgent,
	workspace,
}) => {
	const appSections = organizeAgentApps(subAgent.apps);
	const showApps =
		subAgent.status === "connected" &&
		appSections.some((section) => section.apps.length > 0);

	return (
		<div className="flex flex-col max-w-full relative py-4 border border-dashed border-border rounded">
			<div
				className="absolute -top-2 left-5
				flex items-center gap-2
				bg-surface-primary px-2
				text-xs text-content-secondary"
			>
				<BotIcon size={12} className="mr-1.5" />
				<span>child agent</span>
			</div>

			<header
				className="flex items-center justify-between flex-wrap
				gap-6 px-4 pl-8 leading-6
				md:gap-4"
			>
				<div className="flex items-center gap-4 text-xs text-content-secondary">
					<AgentStatus agent={subAgent} />
					<span
						className="shrink-0
						overflow-hidden text-ellipsis whitespace-nowrap
						text-sm font-semibold text-content-primary"
					>
						{subAgent.name}
					</span>
					{subAgent.execution_isolation && (
						<Badge variant="info" size="sm">
							Isolated execution
						</Badge>
					)}
				</div>
			</header>

			{showApps && (
				<div className="flex flex-col gap-8 px-8 pt-4">
					<section className="flex flex-wrap gap-4 empty:hidden md:justify-start">
						{appSections.map((section, i) => (
							<AgentApps
								key={section.group ?? i}
								section={section}
								agent={subAgent}
								workspace={workspace}
							/>
						))}
					</section>
				</div>
			)}
		</div>
	);
};
