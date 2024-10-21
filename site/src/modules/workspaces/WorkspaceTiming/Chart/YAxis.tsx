import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLProps } from "react";

export const YAxis: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <div css={styles.root} {...props} />;
};

export const YAxisSection: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <section {...props} css={styles.section} />;
};

export const YAxisHeader: FC<HTMLProps<HTMLSpanElement>> = (props) => {
	return <header css={styles.header} {...props} />;
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
		width: "var(--y-axis-width)",
		flexShrink: 0,
	},
	section: (theme) => ({
		"&:not(:first-child)": {
			borderTop: `1px solid ${theme.palette.divider}`,
		},
	}),
	header: (theme) => ({
		height: "var(--header-height)",
		display: "flex",
		alignItems: "center",
		borderBottom: `1px solid ${theme.palette.divider}`,
		fontSize: 10,
		fontWeight: 500,
		color: theme.palette.text.secondary,
		paddingLeft: "var(--section-padding)",
		paddingRight: "var(--section-padding)",
		position: "sticky",
		top: 0,
		background: theme.palette.background.default,
	}),
	labels: {
		margin: 0,
		listStyle: "none",
		display: "flex",
		flexDirection: "column",
		gap: "var(--x-axis-rows-gap)",
		textAlign: "right",
		padding: "var(--section-padding)",
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
