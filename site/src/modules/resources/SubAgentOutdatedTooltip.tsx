import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
} from "api/typesGenerated";
import {
	HelpTooltip,
	HelpTooltipAction,
	HelpTooltipContent,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { Stack } from "components/Stack/Stack";
import { TooltipTrigger } from "components/Tooltip/Tooltip";
import { RotateCcwIcon } from "lucide-react";
import type { FC } from "react";

type SubAgentOutdatedTooltipProps = {
	devcontainer: WorkspaceAgentDevcontainer;
	agent: WorkspaceAgent;
	onUpdate: () => void;
};

export const SubAgentOutdatedTooltip: FC<SubAgentOutdatedTooltipProps> = ({
	devcontainer,
	agent,
	onUpdate,
}) => {
	if (!devcontainer.agent || devcontainer.agent.id !== agent.id) {
		return null;
	}
	if (!devcontainer.dirty) {
		return null;
	}

	// A devcontainer has a pre-created sub agent if subagent_id is present and
	// non-empty. These are defined in Terraform and cannot be rebuilt from the UI.
	const hasPrecreatedSubagent = Boolean(devcontainer.subagent_id);

	const title = "Dev Container Outdated";
	const opener = "This Dev Container is outdated.";
	const text = hasPrecreatedSubagent
		? `${opener} This dev container is managed by your template. Update the template to apply changes.`
		: `${opener} This can happen if you modify your devcontainer.json file after the Dev Container has been created. To fix this, you can rebuild the Dev Container.`;

	return (
		<HelpTooltip>
			<TooltipTrigger className="px-0 py-1 bg-transparent text-inherit border-none opacity-50 hover:opacity-100">
				<span role="status" className="cursor-pointer">
					Outdated
				</span>
			</TooltipTrigger>
			<HelpTooltipContent>
				<Stack spacing={1}>
					<div>
						<HelpTooltipTitle>{title}</HelpTooltipTitle>
						<HelpTooltipText>{text}</HelpTooltipText>
					</div>

					{!hasPrecreatedSubagent && (
						<HelpTooltipLinksGroup>
							<HelpTooltipAction
								icon={RotateCcwIcon}
								onClick={onUpdate}
								ariaLabel="Rebuild Dev Container"
							>
								Rebuild Dev Container
							</HelpTooltipAction>
						</HelpTooltipLinksGroup>
					)}
				</Stack>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
