import IconButton from "@mui/material/IconButton";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { EyeIcon, EyeOffIcon } from "lucide-react";
import { type FC, useState } from "react";

const Language = {
	showLabel: "Show value",
	hideLabel: "Hide value",
};

interface SensitiveValueProps {
	value: string;
}

export const SensitiveValue: FC<SensitiveValueProps> = ({ value }) => {
	const [shouldDisplay, setShouldDisplay] = useState(false);
	const displayValue = shouldDisplay ? value : "••••••••";
	const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel;
	const icon = shouldDisplay ? (
		<EyeOffIcon className="size-icon-xs" />
	) : (
		<EyeIcon className="size-icon-xs" />
	);

	return (
		<div className="flex items-center gap-1">
			<CopyableValue
				value={value}
				className="w-[calc(100%-22px)] overflow-hidden whitespace-nowrap text-ellipsis"
			>
				{displayValue}
			</CopyableValue>
			<Tooltip>
				<TooltipTrigger asChild>
					<IconButton
						className="text-inherit"
						onClick={() => {
							setShouldDisplay((value) => !value);
						}}
						size="small"
						aria-label={buttonLabel}
					>
						{icon}
					</IconButton>
				</TooltipTrigger>
				<TooltipContent side="bottom">{buttonLabel}</TooltipContent>
			</Tooltip>
		</div>
	);
};
