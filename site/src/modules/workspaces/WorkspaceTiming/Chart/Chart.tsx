import type { Interpolation, Theme } from "@emotion/react";
import ChevronRight from "@mui/icons-material/ChevronRight";
import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import type { FC, HTMLProps } from "react";
import React from "react";
import type { BarColors } from "./Bar";
import { YAxisSidePadding, YAxisWidth } from "./YAxis";

export const Chart = (props: HTMLProps<HTMLDivElement>) => {
	return <div css={styles.chart} {...props} />;
};

export const ChartContent: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <div css={styles.content} {...props} />;
};

export const ChartToolbar = (props: HTMLProps<HTMLDivElement>) => {
	return <div css={styles.toolbar} {...props} />;
};

type ChartBreadcrumb = {
	label: string;
	onClick?: () => void;
};

type ChartBreadcrumbsProps = {
	breadcrumbs: ChartBreadcrumb[];
};

export const ChartBreadcrumbs: FC<ChartBreadcrumbsProps> = ({
	breadcrumbs,
}) => {
	return (
		<ul css={styles.breadcrumbs}>
			{breadcrumbs.map((b, i) => {
				const isLast = i === breadcrumbs.length - 1;
				return (
					<React.Fragment key={b.label}>
						<li>
							{isLast ? (
								b.label
							) : (
								<button
									type="button"
									css={styles.breadcrumbButton}
									onClick={b.onClick}
								>
									{b.label}
								</button>
							)}
						</li>
						{!isLast && (
							<li role="presentation">
								<ChevronRight />
							</li>
						)}
					</React.Fragment>
				);
			})}
		</ul>
	);
};

export const ChartSearch = (props: SearchFieldProps) => {
	return <SearchField css={styles.searchField} {...props} />;
};

export type ChartLegend = {
	label: string;
	colors?: BarColors;
};

type ChartLegendsProps = {
	legends: ChartLegend[];
};

export const ChartLegends: FC<ChartLegendsProps> = ({ legends }) => {
	return (
		<ul css={styles.legends}>
			{legends.map((l) => (
				<li key={l.label} css={styles.legend}>
					<div
						css={[
							styles.legendSquare,
							{
								borderColor: l.colors?.stroke,
								backgroundColor: l.colors?.fill,
							},
						]}
					/>
					{l.label}
				</li>
			))}
		</ul>
	);
};

const styles = {
	chart: {
		height: "100%",
		display: "flex",
		flexDirection: "column",
	},
	content: (theme) => ({
		display: "flex",
		alignItems: "stretch",
		fontSize: 12,
		fontWeight: 500,
		overflow: "auto",
		flex: 1,
		scrollbarColor: `${theme.palette.divider} ${theme.palette.background.default}`,
		scrollbarWidth: "thin",
	}),
	toolbar: (theme) => ({
		borderBottom: `1px solid ${theme.palette.divider}`,
		fontSize: 12,
		display: "flex",
		flexAlign: "stretch",
	}),
	breadcrumbs: (theme) => ({
		listStyle: "none",
		margin: 0,
		width: YAxisWidth,
		padding: YAxisSidePadding,
		display: "flex",
		alignItems: "center",
		gap: 4,
		lineHeight: 1,
		flexShrink: 0,

		"& li": {
			display: "block",

			"&[role=presentation]": {
				lineHeight: 0,
			},
		},

		"& li:first-child": {
			color: theme.palette.text.secondary,
		},

		"& li[role=presentation]": {
			color: theme.palette.text.secondary,

			"& svg": {
				width: 14,
				height: 14,
			},
		},
	}),
	breadcrumbButton: (theme) => ({
		background: "none",
		padding: 0,
		border: "none",
		fontSize: "inherit",
		color: "inherit",
		cursor: "pointer",

		"&:hover": {
			color: theme.palette.text.primary,
		},
	}),
	searchField: (theme) => ({
		flex: "1",

		"& fieldset": {
			border: 0,
			borderRadius: 0,
			borderLeft: `1px solid ${theme.palette.divider} !important`,
		},

		"& .MuiInputBase-root": {
			height: "100%",
			fontSize: 12,
		},
	}),
	legends: {
		listStyle: "none",
		margin: 0,
		padding: 0,
		display: "flex",
		alignItems: "center",
		gap: 24,
		paddingRight: YAxisSidePadding,
	},
	legend: {
		fontWeight: 500,
		display: "flex",
		alignItems: "center",
		gap: 8,
		lineHeight: 1,
	},
	legendSquare: (theme) => ({
		width: 18,
		height: 18,
		borderRadius: 4,
		border: `1px solid ${theme.palette.divider}`,
		backgroundColor: theme.palette.background.default,
	}),
} satisfies Record<string, Interpolation<Theme>>;
