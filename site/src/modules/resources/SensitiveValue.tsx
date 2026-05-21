import { EyeIcon, EyeOffIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import { CopyableValue } from "#/components/CopyableValue/CopyableValue";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

interface SensitiveValueProps {
	value: string;
}

export const SensitiveValue: FC<SensitiveValueProps> = ({ value }) => {
	const [shouldDisplay, setShouldDisplay] = useState(false);
	const displayValue = shouldDisplay ? value : "••••••••";
	const buttonLabel = shouldDisplay ? "Hide value" : "Show value";
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
					<Button
						onClick={() => {
							setShouldDisplay((value) => !value);
						}}
						size="icon"
						variant="subtle"
						className="size-6"
						aria-label={buttonLabel}
					>
						{icon}
					</Button>
				</TooltipTrigger>
				<TooltipContent side="bottom">{buttonLabel}</TooltipContent>
			</Tooltip>
		</div>
	);
};
