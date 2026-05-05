import type { FC } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { docs } from "#/utils/docs";

export const RolesHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger size="small" />
			<HelpPopoverContent>
				<HelpPopoverTitle>What is a role?</HelpPopoverTitle>
				<HelpPopoverText>
					Coder role-based access control (RBAC) provides fine-grained access
					management. View our docs on how to use the available roles.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/admin/users/groups-roles")}>
						User Roles
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};

export const GroupsHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger size="small" />
			<HelpPopoverContent>
				<HelpPopoverTitle>What is a group?</HelpPopoverTitle>
				<HelpPopoverText>
					Groups can be used with template RBAC to give groups of users access
					to specific templates. View our docs on how to use groups.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/admin/users/groups-roles")}>
						Groups
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};

export const AiAddonHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger size="small" />
			<HelpPopoverContent>
				<HelpPopoverTitle>What is the AI add-on?</HelpPopoverTitle>
				<HelpPopoverText>
					Users with access to AI features like AI Bridge or Tasks who are
					actively consuming a seat.
				</HelpPopoverText>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
