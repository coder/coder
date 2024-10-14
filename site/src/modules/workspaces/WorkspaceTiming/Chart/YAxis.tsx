import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLProps } from "react";
import { XAxisLabelsHeight, XAxisRowsGap } from "./constants";

export const YAxisWidth = 200;
export const YAxisSidePadding = 16;

export const YAxis: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <div css={styles.root} {...props} />;
};

export const YAxisSection: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <section {...props} />;
};

export const YAxisCaption: FC<HTMLProps<HTMLSpanElement>> = (props) => {
	return <span css={styles.caption} {...props} />;
};

export const YAxisLabels: FC<HTMLProps<HTMLUListElement>> = (props) => {
	return <ul css={styles.labels} {...props} />;
};

type YAxisLabelProps = Omit<HTMLProps<HTMLLIElement>, "id"> & {
	id: string;
};

export const YAxisLabel: FC<YAxisLabelProps> = ({ id, ...props }) => {
	return (
		<li {...props} css={styles.label} id={encodeURIComponent(id)}>
			<span>{props.children}</span>
		</li>
	);
};

const styles = {
	root: {
		width: YAxisWidth,
		flexShrink: 0,
	},
	caption: (theme) => ({
		height: XAxisLabelsHeight,
		display: "flex",
		alignItems: "center",
		justifyContent: "end",
		borderBottom: `1px solid ${theme.palette.divider}`,
		fontSize: 10,
		fontWeight: 500,
		color: theme.palette.text.secondary,
		paddingLeft: YAxisSidePadding,
		paddingRight: YAxisSidePadding,
		position: "sticky",
		top: 0,
		background: theme.palette.background.default,
	}),
	labels: {
		margin: 0,
		listStyle: "none",
		display: "flex",
		flexDirection: "column",
		gap: XAxisRowsGap,
		textAlign: "right",
		padding: YAxisSidePadding,
	},
	label: {
		display: "flex",
		alignItems: "center",

		"& > *": {
			display: "block",
			width: "100%",
			overflow: "hidden",
			textOverflow: "ellipsis",
			whiteSpace: "nowrap",
		},
	},
} satisfies Record<string, Interpolation<Theme>>;
