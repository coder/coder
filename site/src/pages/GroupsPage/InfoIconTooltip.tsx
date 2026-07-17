import { InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverText,
} from "#/components/HelpPopover/HelpPopover";

/** An (i) info tooltip. `className` sets the icon color. */
export const InfoIconTooltip: FC<{
	message: ReactNode;
	className?: string;
}> = ({ message, className = "text-content-secondary" }) => (
	<HelpPopover>
		<HelpPopoverIconTrigger size="small" hoverEffect={false}>
			<InfoIcon className={className} />
		</HelpPopoverIconTrigger>
		<HelpPopoverContent>
			<HelpPopoverText>{message}</HelpPopoverText>
		</HelpPopoverContent>
	</HelpPopover>
);
