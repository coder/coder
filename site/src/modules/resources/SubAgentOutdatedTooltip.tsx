import { RotateCcwIcon } from "lucide-react";
import type { FC } from "react";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
} from "#/api/typesGenerated";
import {
	HelpPopover,
	HelpPopoverAction,
	HelpPopoverContent,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
	HelpPopoverTrigger,
} from "#/components/HelpPopover/HelpPopover";

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

	return (
		<HelpPopover>
			<HelpPopoverTrigger className="px-0 py-1 bg-transparent text-inherit border-none opacity-50 hover:opacity-100">
				<span role="status" className="cursor-pointer">
					Outdated
				</span>
			</HelpPopoverTrigger>
			<HelpPopoverContent>
				<div className="flex flex-col gap-2">
					<div>
						<HelpPopoverTitle>Dev Container Outdated</HelpPopoverTitle>
						<HelpPopoverText>
							This Dev Container is outdated. This can happen if you modify your
							devcontainer.json file after the Dev Container has been created.
							To fix this, you can rebuild the Dev Container.
						</HelpPopoverText>
					</div>

					<HelpPopoverLinksGroup>
						<HelpPopoverAction
							icon={RotateCcwIcon}
							onClick={onUpdate}
							ariaLabel="Rebuild Dev Container"
						>
							Rebuild Dev Container
						</HelpPopoverAction>
					</HelpPopoverLinksGroup>
				</div>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
