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

export const AuditHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />

			<HelpPopoverContent>
				<HelpPopoverTitle>What is an audit log?</HelpPopoverTitle>
				<HelpPopoverText>
					An audit log is a record of events and changes made throughout a
					system.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/admin/security/audit-logs")}>
						Events we track
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
