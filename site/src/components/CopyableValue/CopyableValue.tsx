import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import type { FC, HTMLAttributes } from "react";

type TooltipSide = "top" | "right" | "bottom" | "left";

interface CopyableValueProps extends HTMLAttributes<HTMLSpanElement> {
	value: string;
	side?: TooltipSide;
}

export const CopyableValue: FC<CopyableValueProps> = ({
	value,
	side = "bottom",
	children,
	className,
	...attrs
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard();
	const clickableProps = useClickable<HTMLSpanElement>(() => {
		copyToClipboard(value);
	});

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						{...attrs}
						{...clickableProps}
						className={`cursor-pointer ${className || ""}`}
					>
						{children}
					</span>
				</TooltipTrigger>
				<TooltipContent side={side}>
					{showCopiedSuccess ? "Copied!" : "Click to copy"}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
