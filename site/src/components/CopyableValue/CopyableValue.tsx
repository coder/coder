import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import { type FC, type HTMLAttributes, useRef, useState } from "react";
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
	const [showCopiedText, setShowCopiedText] = useState(false);
	const prevCopiedRef = useRef(false);
	const timeoutRef = useRef<number | undefined>(undefined);

	const clickableProps = useClickable<HTMLSpanElement>(() => {
		copyToClipboard(value);
		setTooltipOpen(true);
	});

	if (showCopiedSuccess !== prevCopiedRef.current) {
		prevCopiedRef.current = showCopiedSuccess;

		if (showCopiedSuccess) {
			setShowCopiedText(true);
		} else {
			setTooltipOpen(false);
			clearTimeout(timeoutRef.current);
			// showCopiedText should reset after tooltip is closed to avoid a flash of the text
			timeoutRef.current = window.setTimeout(
				() => setShowCopiedText(false),
				100,
			);
		}
	}

	return (
		<Tooltip open={tooltipOpen} onOpenChange={setTooltipOpen}>
			<TooltipTrigger asChild>
				<span
					{...attrs}
					{...clickableProps}
					className={cn("cursor-pointer", className)}
				>
					{children}
				</span>
			</TooltipTrigger>
			<TooltipContent side={side}>
				{showCopiedText ? "Copied!" : "Click to copy"}
			</TooltipContent>
		</Tooltip>
	);
};
