import type { FC } from "react";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "#/components/HelpTooltip/HelpTooltip";
import { docs } from "#/utils/docs";

export const AuditHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />

			<HelpTooltipContent>
				<HelpTooltipTitle>{"What is an audit log?"}</HelpTooltipTitle>
				<HelpTooltipText>
					{
						"An audit log is a record of events and changes made throughout a system."
					}
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/admin/security/audit-logs")}>
						{"Events we track"}
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
