import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";

interface CopyableValueProps extends HTMLAttributes<HTMLSpanElement> {
	value: string;
	placement?: TooltipProps["placement"];
	PopperProps?: TooltipProps["PopperProps"];
}

export const CopyableValue: FC<CopyableValueProps> = ({
	value,
	placement = "bottom-start",
	PopperProps,
	children,
	className,
	...attrs
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard({
		textToCopy: value,
	});
	const clickableProps = useClickable<HTMLSpanElement>(copyToClipboard);

	return (
		<Tooltip
			title={showCopiedSuccess ? "Copied!" : "Click to copy"}
			placement={placement}
			PopperProps={PopperProps}
		>
			<span
				{...attrs}
				{...clickableProps}
				className={cn(
					"flex flex-row gap-1 items-center cursor-pointer",
					className,
				)}
			>
				{children}
			</span>
		</Tooltip>
	);
};
