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

const Language = {
	title: "Why are some events missing?",
	body: "The connection log is a best-effort log of workspace access. Some events are reported by workspace agents, and receipt of these events by the server is not guaranteed.",
	docs: "Connection log documentation",
};

export const ConnectionLogHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />

			<HelpPopoverContent>
				<HelpPopoverTitle>{Language.title}</HelpPopoverTitle>
				<HelpPopoverText>{Language.body}</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/admin/monitoring/connection-logs")}>
						{Language.docs}
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
