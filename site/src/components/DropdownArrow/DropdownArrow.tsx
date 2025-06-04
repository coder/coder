import type { Interpolation, Theme } from "@emotion/react";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import type { FC } from "react";

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
			css={[styles.base, margin && styles.withMargin]}
			style={{ color }}
		/>
	);
};

const styles = {
	base: {
		color: "currentcolor",
		width: 16,
		height: 16,
	},

	withMargin: {
		marginLeft: 8,
	},
} satisfies Record<string, Interpolation<Theme>>;
