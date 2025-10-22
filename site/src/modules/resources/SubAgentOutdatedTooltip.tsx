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

	const title = "Dev Container Outdated";
	const opener = "This Dev Container is outdated.";
	const text = `${opener} This can happen if you modify your devcontainer.json file after the Dev Container has been created. To fix this, you can rebuild the Dev Container.`;

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

					<HelpTooltipLinksGroup>
						<HelpTooltipAction
							icon={RotateCcwIcon}
							onClick={onUpdate}
							ariaLabel="Rebuild Dev Container"
						>
							Rebuild Dev Container
						</HelpTooltipAction>
					</HelpTooltipLinksGroup>
				</Stack>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
