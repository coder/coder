import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { TooltipMessage, TooltipTitle } from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

export const RolesHelpPopover: FC = () => {
	return (
		<InfoTooltip size="small">
			<TooltipTitle>What is a role?</TooltipTitle>
			<TooltipMessage>
				Coder role-based access control (RBAC) provides fine-grained access
				management. View our docs on how to use the available roles.
				<br />
				<Link size="sm" href={docs("/admin/users/groups-roles")}>
					User Roles
				</Link>
			</TooltipMessage>
		</InfoTooltip>
	);
};

export const GroupsHelpPopover: FC = () => {
	return (
		<InfoTooltip size="small">
			<TooltipTitle>What is a group?</TooltipTitle>
			<TooltipMessage>
				Groups can be used with template RBAC to give groups of users access to
				specific templates. View our docs on how to use groups.
				<br />
				<Link size="sm" href={docs("/admin/users/groups-roles")}>
					Groups
				</Link>
			</TooltipMessage>
		</InfoTooltip>
	);
};
