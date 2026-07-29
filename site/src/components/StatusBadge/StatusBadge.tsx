import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { themeRoleToBadgeVariant } from "#/components/Badge/themeRoleToBadgeVariant";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { httpStatusColor } from "#/utils/http";

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
