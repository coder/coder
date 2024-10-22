import type { Interpolation, Theme } from "@emotion/react";
import ChevronRight from "@mui/icons-material/ChevronRight";
import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import type { FC, HTMLProps } from "react";
import React, { useEffect, useRef } from "react";
import type { BarColors } from "./Bar";

export const Chart = (props: HTMLProps<HTMLDivElement>) => {
	return <div css={styles.chart} {...props} />;
};

export const ChartContent: FC<HTMLProps<HTMLDivElement>> = (props) => {
	const contentRef = useRef<HTMLDivElement>(null);

	// Display a scroll mask when the content is scrollable and update its
	// position on scroll. Remove the mask when the scroll reaches the bottom to
	// ensure the last item is visible.
	useEffect(() => {
		const contentEl = contentRef.current;
		if (!contentEl) return;

		const hasScroll = contentEl.scrollHeight > contentEl.clientHeight;
		contentEl.style.setProperty("--scroll-mask-opacity", hasScroll ? "1" : "0");

		const handler = () => {
			if (!hasScroll) {
				return;
			}
			contentEl.style.setProperty("--scroll-top", `${contentEl.scrollTop}px`);
			const isBottom =
				contentEl.scrollTop + contentEl.clientHeight >= contentEl.scrollHeight;
			contentEl.style.setProperty(
				"--scroll-mask-opacity",
				isBottom ? "0" : "1",
			);
		};
		contentEl.addEventListener("scroll", handler);
		return () => contentEl.removeEventListener("scroll", handler);
	}, []);

	return <div css={styles.content} {...props} ref={contentRef} />;
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
		"--header-height": "40px",
		"--section-padding": "16px",
		"--x-axis-rows-gap": "20px",
		"--y-axis-width": "200px",

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
		position: "relative",

		"&:before": {
			content: "''",
			position: "absolute",
			bottom: "calc(-1 * var(--scroll-top, 0px))",
			width: "100%",
			height: 100,
			background: `linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, ${theme.palette.background.default} 81.93%)`,
			opacity: "var(--scroll-mask-opacity)",
			zIndex: 1,
			transition: "opacity 0.2s",
			pointerEvents: "none",
		},
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
		width: "var(--y-axis-width)",
		padding: "var(--section-padding)",
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
		paddingRight: "var(--section-padding)",
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
