import { cn } from "cn";
import {
	containerWidth,
	containerWidthMedium,
	sidePadding,
} from "#/theme/constants";

export type Size = "regular" | "medium" | "condensed" | "small";

const widthBySize: Record<Size, number> = {
	regular: containerWidth,
	medium: containerWidthMedium,
	condensed: containerWidth / 2,
	small: containerWidth / 3,
};

type MarginsProps = React.JSX.IntrinsicElements["div"] & {
	size?: Size;
};

export const Margins: React.FC<MarginsProps> = ({
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
