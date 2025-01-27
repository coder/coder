import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import type { FC } from "react";
import { docs } from "utils/docs";

export const IdpSyncHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger />
			<HelpTooltipContent>
				<HelpTooltipTitle>What is IdP Sync?</HelpTooltipTitle>
				<HelpTooltipText>
					View the current mappings between your external OIDC provider and
					Coder. Use the Coder CLI to configure these mappings.
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/admin/users/idp-sync")}>
						Configure IdP Sync
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
