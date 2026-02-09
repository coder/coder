import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIcon,
	HelpTooltipIconTrigger,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { FC, ReactNode } from "react";
import type { ThemeRole } from "theme/roles";
import { cn } from "utils/cn";

interface InfoTooltipProps {
	type?: ThemeRole;
	title: ReactNode;
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
	inactive: "text-highlight-grey",
	preview: "text-highlight-purple",
};

export const InfoTooltip: FC<InfoTooltipProps> = ({
	title,
	message,
	type = "info",
}) => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger size="small" hoverEffect={false}>
				<HelpTooltipIcon className={cn(tooltipColorClasses[type])} />
			</HelpTooltipIconTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>{title}</HelpTooltipTitle>
				<HelpTooltipText>{message}</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
