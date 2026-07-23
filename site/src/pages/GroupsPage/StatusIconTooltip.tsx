import { InfoIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverText,
} from "#/components/HelpPopover/HelpPopover";

type StatusIconKind = "info" | "warning";

const statusIcon: Record<StatusIconKind, ReactNode> = {
	info: <InfoIcon className="text-content-secondary" />,
	warning: <TriangleAlertIcon className="text-content-warning" />,
};

/** A popover tooltip anchored to a status icon, styled per `kind`. */
export const StatusIconTooltip: FC<{
	message: ReactNode;
	kind?: StatusIconKind;
}> = ({ message, kind = "info" }) => (
	<HelpPopover>
		<HelpPopoverIconTrigger size="small" hoverEffect={false}>
			{statusIcon[kind]}
		</HelpPopoverIconTrigger>
		<HelpPopoverContent>
			<HelpPopoverText>{message}</HelpPopoverText>
		</HelpPopoverContent>
	</HelpPopover>
);
