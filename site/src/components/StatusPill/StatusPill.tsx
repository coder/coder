import { Pill } from "components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import { httpStatusColor } from "utils/http";

interface StatusPillProps {
	code: number;
	isHttpCode: boolean;
	label?: string;
}

export const StatusPill: FC<StatusPillProps> = ({
	code,
	isHttpCode,
	label,
}) => {
	const pill = (
		<Pill
			className="text-[10px] h-5 px-2.5 font-semibold"
			type={
				isHttpCode ? httpStatusColor(code) : code === 0 ? "success" : "error"
			}
		>
			{code.toString()}
		</Pill>
	);
	if (!label) {
		return pill;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{pill}</TooltipTrigger>
			<TooltipContent>{label}</TooltipContent>
		</Tooltip>
	);
};
