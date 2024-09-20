import type { FC, HTMLProps, ReactNode } from "react";
import type { Interpolation, Theme } from "@emotion/react";
import { columnWidth, contentSidePadding, XAxisHeight } from "./constants";

type XValuesProps = HTMLProps<HTMLDivElement> & {
	labels: ReactNode[];
};

export const XAxis: FC<XValuesProps> = ({ labels, ...htmlProps }) => {
	return (
		<div css={styles.row} {...htmlProps}>
			{labels.map((l, i) => (
				<div
					// biome-ignore lint/suspicious/noArrayIndexKey: we are iterating over a ReactNode so we don't have another prop to use as key
					key={i}
					css={[
						styles.label,
						{
							// To centralize the labels between columns, we need to:
							// 1. Set the label width to twice the column width.
							// 2. Shift the label to the left by half of the column width.
							// Note: This adjustment is not applied to the first element,
							// as the 0 label/value is not displayed in the chart.
							width: columnWidth * 2,
							"&:not(:first-child)": {
								marginLeft: -columnWidth,
							},
						},
					]}
				>
					{l}
				</div>
			))}
		</div>
	);
};

const styles = {
	row: (theme) => ({
		display: "flex",
		width: "fit-content",
		alignItems: "center",
		borderBottom: `1px solid ${theme.palette.divider}`,
		height: XAxisHeight,
		padding: `0px ${contentSidePadding}px`,
		minWidth: "100%",
		flexShrink: 0,
		position: "sticky",
		top: 0,
		zIndex: 1,
		backgroundColor: theme.palette.background.default,
	}),
	label: (theme) => ({
		display: "flex",
		justifyContent: "center",
		flexShrink: 0,
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
