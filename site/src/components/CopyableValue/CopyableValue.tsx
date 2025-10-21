import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import { type FC, type HTMLAttributes, useState } from "react";
import { cn } from "utils/cn";

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
	const [tooltipOpen, setTooltipOpen] = useState(false);
	const [isFocused, setIsFocused] = useState(false);
	const clickableProps = useClickable<HTMLSpanElement>(() => {
		copyToClipboard(value);
		setTooltipOpen(true);
	});

	return (
		<TooltipProvider delayDuration={100}>
			<Tooltip
				open={tooltipOpen}
				onOpenChange={(next) => {
					// Always keep the tooltip open when in focus to handle issues when onOpenChange is unexpectedly false
					if (!next && isFocused) return;
					setTooltipOpen(next);
				}}
			>
				<TooltipTrigger asChild>
					<span
						{...attrs}
						{...clickableProps}
						role="button"
						tabIndex={0}
						onMouseEnter={() => {
							setIsFocused(true);
							setTooltipOpen(true);
						}}
						onMouseLeave={() => {
							setTooltipOpen(false);
						}}
						onFocus={() => {
							setIsFocused(true);
						}}
						onBlur={() => {
							setTooltipOpen(false);
						}}
						className={cn("cursor-pointer", className)}
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
