import {
	MonitorDotIcon,
	MonitorIcon,
	MonitorPauseIcon,
	MonitorXIcon,
} from "lucide-react";
import type { FC } from "react";
import type { DisplayWorkspaceStatusType } from "#/utils/workspace";

const iconMap: Record<
	DisplayWorkspaceStatusType,
	FC<{ className?: string }>
> = {
	success: MonitorIcon,
	active: MonitorDotIcon,
	inactive: MonitorPauseIcon,
	error: MonitorXIcon,
	danger: MonitorXIcon,
	warning: MonitorXIcon,
};

export const StatusIcon: FC<{
	type: DisplayWorkspaceStatusType;
	className?: string;
}> = ({ type, className = "size-3" }) => {
	const Icon = iconMap[type];
	return <Icon className={className} />;
};
