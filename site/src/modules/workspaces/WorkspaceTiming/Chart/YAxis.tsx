import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLProps } from "react";
import { barsSpacing, XAxisHeight } from "./constants";

// Predicting the caption height is necessary to add appropriate spacing to the
// grouped bars, ensuring alignment with the sidebar labels.
export const YAxisCaptionHeight = 20;

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

export const YAxisLabel: FC<HTMLProps<HTMLLIElement>> = (props) => {
	return <li {...props} css={styles.label} />;
};

const styles = {
	root: {
		width: 200,
		flexShrink: 0,
		padding: 16,
		paddingTop: XAxisHeight,
	},
	caption: (theme) => ({
		height: YAxisCaptionHeight,
		display: "flex",
		alignItems: "center",
		fontSize: 10,
		fontWeight: 500,
		color: theme.palette.text.secondary,
	}),
	labels: {
		margin: 0,
		padding: 0,
		listStyle: "none",
		display: "flex",
		flexDirection: "column",
		gap: barsSpacing,
		textAlign: "right",
	},
	label: {
		display: "block",
		maxWidth: "100%",
		overflow: "hidden",
		textOverflow: "ellipsis",
		whiteSpace: "nowrap",
	},
} satisfies Record<string, Interpolation<Theme>>;
