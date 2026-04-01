import type { FC, ReactNode } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIcon,
	HelpPopoverIconTrigger,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import type { ThemeRole } from "#/theme/roles";
import { cn } from "#/utils/cn";

interface InfoTooltipProps {
	type?: ThemeRole;
	title?: ReactNode;
	message: ReactNode;
}

const tooltipColorClasses: Record<ThemeRole, string> = {
	info: "text-content-secondary",
	error: "text-content-destructive",
	warning: "text-content-warning",
	notice: "text-content-link",
	success: "text-content-success",
	danger: "text-content-destructive",
	active: "text-content-link",
	inactive: "text-content-secondary",
	preview: "text-highlight-purple",
};

export const InfoTooltip: FC<InfoTooltipProps> = ({
	title,
	message,
	type = "info",
}) => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger size="small" hoverEffect={false}>
				<HelpPopoverIcon className={cn(tooltipColorClasses[type])} />
			</HelpPopoverIconTrigger>
			<HelpPopoverContent>
				{title && <HelpPopoverTitle>{title}</HelpPopoverTitle>}
				<HelpPopoverText>{message}</HelpPopoverText>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
