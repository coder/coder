import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const RolesHelpPopover: FC = () => {
	return (
		<InfoTooltip
			size="small"
			title="What is a role?"
			message={
				<>
					Coder role-based access control (RBAC) provides fine-grained access
					management. View our docs on how to use the available roles.
					<br />
					<Link size="sm" href={docs("/admin/users/groups-roles")}>
						User Roles
					</Link>
				</>
			}
		/>
	);
};

export const GroupsHelpPopover: FC = () => {
	return (
		<InfoTooltip
			size="small"
			title="What is a group?"
			message={
				<>
					Groups can be used with template RBAC to give groups of users access
					to specific templates. View our docs on how to use groups.
					<br />
					<Link size="sm" href={docs("/admin/users/groups-roles")}>
						Groups
					</Link>
				</>
			}
		/>
	);
};
