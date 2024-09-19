import type { FC, HTMLProps } from "react";
import { Label } from "./Label";
import type { Interpolation, Theme } from "@emotion/react";

type XValuesProps = HTMLProps<HTMLDivElement> & {
	values: string[];
	columnWidth: number;
};

export const XValues: FC<XValuesProps> = ({
	values,
	columnWidth,
	...htmlProps
}) => {
	return (
		<div css={styles.row} {...htmlProps}>
			{values.map((v) => (
				<div
					key={v}
					css={[
						styles.cell,
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
					<Label color="secondary">{v}</Label>
				</div>
			))}
		</div>
	);
};

const styles = {
	row: {
		display: "flex",
		width: "fit-content",
	},
	cell: {
		display: "flex",
		justifyContent: "center",
		flexShrink: 0,
	},
} satisfies Record<string, Interpolation<Theme>>;
