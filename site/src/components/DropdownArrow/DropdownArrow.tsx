import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

interface ArrowProps {
	margin?: boolean;
	close?: boolean;
}

export const DropdownArrow: FC<ArrowProps> = ({ margin = true, close }) => {
	const Arrow = close ? ChevronUpIcon : ChevronDownIcon;

	return (
		<Arrow
			aria-label={close ? "close-dropdown" : "open-dropdown"}
			className={cn("text-current", margin && "ml-2")}
		/>
	);
};
