import type { FC, JSX } from "react";
import {
	containerWidth,
	containerWidthMedium,
	sidePadding,
} from "theme/constants";

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
	...divProps
}) => {
	const maxWidth = widthBySize[size];
	return (
		<div
			{...divProps}
			style={{
				...(maxWidth ? { maxWidth: `${maxWidth}px` } : {}),
				...(sidePadding ? { padding: `${sidePadding}px` } : {}),
			}}
			className="mx-auto w-full"
		>
			{children}
		</div>
	);
};
