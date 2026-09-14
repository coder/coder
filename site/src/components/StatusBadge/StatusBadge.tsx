import type { FC } from "react";
import { Badge, type BadgeProps } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ThemeRole } from "#/theme/roles";
import { httpStatusColor } from "#/utils/http";

function themeRoleToBadgeVariant(
	role: ThemeRole | "muted",
): NonNullable<BadgeProps["variant"]> {
	switch (role) {
		case "success":
			return "green";
		case "error":
			return "destructive";
		case "warning":
		case "danger":
			return "warning";
		case "active":
		case "notice":
			return "info";
		case "preview":
			return "purple";
		default:
			return "default";
	}
}

interface StatusBadgeProps {
	code: number;
	isHttpCode: boolean;
	label?: string;
}

export const StatusBadge: FC<StatusBadgeProps> = ({
	code,
	isHttpCode,
	label,
}) => {
	const role = isHttpCode
		? httpStatusColor(code)
		: code === 0
			? "success"
			: "error";
	const badge = (
		<Badge
			className="text-[10px] h-5 px-2.5 font-semibold"
			variant={themeRoleToBadgeVariant(role)}
		>
			{code.toString()}
		</Badge>
	);
	if (!label) {
		return badge;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{badge}</TooltipTrigger>
			<TooltipContent>{label}</TooltipContent>
		</Tooltip>
	);
};
