import type { WorkspaceAgent } from "api/typesGenerated";
import {
	HelpTooltip,
	HelpTooltipAction,
	HelpTooltipContent,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Stack } from "components/Stack/Stack";
import { RotateCcwIcon } from "lucide-react";
import { type FC, useState } from "react";
import { agentVersionStatus } from "../../utils/workspace";

type AgentOutdatedTooltipProps = {
	agent: WorkspaceAgent;
	serverVersion: string;
	status: agentVersionStatus;
	onUpdate: () => void;
};

export const AgentOutdatedTooltip: FC<AgentOutdatedTooltipProps> = ({
	agent,
	serverVersion,
	status,
	onUpdate,
}) => {
	const [isOpen, setIsOpen] = useState(false);

	const title =
		status === agentVersionStatus.Outdated
			? "Agent outdated"
			: "Agent deprecated";
	const opener =
		status === agentVersionStatus.Outdated
			? "This agent is an older version than the Coder server."
			: "This agent is using a deprecated version of the API.";
	const text = `${opener} This can happen after you update Coder with running workspaces. To fix this, you can stop and start the workspace.`;

	return (
		<HelpTooltip open={isOpen} onOpenChange={setIsOpen}>
			<HelpTooltipTrigger asChild>
				<span role="status" className="cursor-pointer">
					{status === agentVersionStatus.Outdated ? "Outdated" : "Deprecated"}
				</span>
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<Stack spacing={1}>
					<div>
						<HelpTooltipTitle>{title}</HelpTooltipTitle>
						<HelpTooltipText>{text}</HelpTooltipText>
					</div>

					<Stack spacing={0.5}>
						<span className="font-semibold text-content-primary">
							Agent version
						</span>
						<span>{agent.version}</span>
					</Stack>

					<Stack spacing={0.5}>
						<span className="font-semibold text-content-primary">
							Server version
						</span>
						<span>{serverVersion}</span>
					</Stack>

					<HelpTooltipLinksGroup>
						<HelpTooltipAction
							icon={RotateCcwIcon}
							onClick={() => {
								onUpdate();
								setIsOpen(false);
							}}
							ariaLabel="Update workspace"
						>
							Update workspace
						</HelpTooltipAction>
					</HelpTooltipLinksGroup>
				</Stack>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
