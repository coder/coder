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
import { TooltipTrigger } from "components/Tooltip/Tooltip";
import { RotateCcwIcon } from "lucide-react";
import type { FC } from "react";
import { isTerraformDefined } from "./devcontainerUtils";

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

	const hasPrecreatedSubagent = isTerraformDefined(devcontainer);

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
				<div className="flex flex-col gap-2">
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
				</div>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
