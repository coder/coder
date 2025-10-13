import {
	Tooltip,
	TooltipContent,
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
	role,
	tabIndex,
	onClick,
	onKeyDown,
	onKeyUp,
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
		<TooltipProvider>
			<Tooltip
				open={tooltipOpen}
				onOpenChange={(shouldBeOpen) => {
					// Always keep the tooltip open when in focus to handle issues when onOpenChange is unexpectedly false
					if (!shouldBeOpen && isFocused) return;
					setTooltipOpen(shouldBeOpen);
				}}
			>
				<TooltipTrigger asChild>
					<span
						ref={clickableProps.ref}
						{...attrs}
						className={cn("cursor-pointer", className)}
						role={role ?? clickableProps.role}
						tabIndex={tabIndex ?? clickableProps.tabIndex}
						onClick={(event) => {
							clickableProps.onClick(event);
							onClick?.(event);
						}}
						onKeyDown={(event) => {
							clickableProps.onKeyDown(event);
							onKeyDown?.(event);
						}}
						onKeyUp={(event) => {
							clickableProps.onKeyUp(event);
							onKeyUp?.(event);
						}}
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
