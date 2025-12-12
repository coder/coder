import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

interface ArrowProps {
	margin?: boolean;
	color?: string;
	close?: boolean;
}

export const DropdownArrow: FC<ArrowProps> = ({
	margin = true,
	color,
	close,
}) => {
	const Arrow = close ? ChevronUpIcon : ChevronDownIcon;

	return (
		<Arrow
			aria-label={close ? "close-dropdown" : "open-dropdown"}
			className={cn("size-4 text-current", margin && "ml-2")}
			style={{ color }}
		/>
	);
};
