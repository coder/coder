import type { FC, JSX } from "react";
import {
	containerWidth,
	containerWidthMedium,
	sidePadding,
} from "#/theme/constants";
import { cn } from "#/utils/cn";

type Size = "regular" | "medium" | "small";

const widthBySize: Record<Size, number> = {
	regular: containerWidth,
	medium: containerWidthMedium,
	small: containerWidth / 3,
};

type MarginsProps = JSX.IntrinsicElements["div"] & {
	size?: Size;
};

export const Margins: FC<MarginsProps> = ({
	size = "regular",
	children,
	className,
	...divProps
}) => {
	const maxWidth = widthBySize[size];
	return (
		<div
			{...divProps}
			style={{
				maxWidth: maxWidth,
				paddingLeft: sidePadding,
				paddingRight: sidePadding,
			}}
			className={cn("mx-auto w-full", className)}
		>
			{children}
		</div>
	);
};
