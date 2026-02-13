import type { CSSObject } from "@emotion/react";

type StackProps = React.ComponentPropsWithRef<"div"> & {
	className?: string;
	direction?: "column" | "row";
	spacing?: number;
	alignItems?: CSSObject["alignItems"];
	justifyContent?: CSSObject["justifyContent"];
	wrap?: CSSObject["flexWrap"];
};

/**
 * @deprecated Stack component is deprecated. Use Tailwind flex utilities instead.
 */
export const Stack: React.FC<StackProps> = (props) => {
	const {
		children,
		direction = "column",
		spacing = 2,
		alignItems,
		justifyContent,
		wrap,
		...divProps
	} = props;

	return (
		<div
			{...divProps}
			css={{
				display: "flex",
				flexDirection: direction,
				gap: spacing * 8,
				alignItems: alignItems,
				justifyContent: justifyContent,
				flexWrap: wrap,
				maxWidth: "100%",
			}}
		>
			{children}
		</div>
	);
};
