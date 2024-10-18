import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import MUITooltip, {
	type TooltipProps as MUITooltipProps,
} from "@mui/material/Tooltip";
import type { FC, HTMLProps } from "react";
import { Link, type LinkProps } from "react-router-dom";

export type TooltipProps = MUITooltipProps;

export const Tooltip: FC<TooltipProps> = (props) => {
	const theme = useTheme();

	return (
		<MUITooltip
			classes={{
				tooltip: css(styles.tooltip(theme)),
				...props.classes,
			}}
			{...props}
		/>
	);
};

export const TooltipTitle: FC<HTMLProps<HTMLSpanElement>> = (props) => {
	return <span css={styles.title} {...props} />;
};

export const TooltipShortDescription: FC<HTMLProps<HTMLSpanElement>> = (
	props,
) => {
	return <span css={styles.shortDesc} {...props} />;
};

export const TooltipLink: FC<LinkProps> = (props) => {
	return (
		<Link {...props} css={styles.link}>
			<OpenInNewOutlined />
			{props.children}
		</Link>
	);
};

const styles = {
	tooltip: (theme) => ({
		backgroundColor: theme.palette.background.default,
		border: `1px solid ${theme.palette.divider}`,
		maxWidth: "max-content",
		borderRadius: 8,
		display: "flex",
		flexDirection: "column",
		fontWeight: 500,
		fontSize: 12,
		color: theme.palette.text.secondary,
		gap: 4,
	}),
	title: (theme) => ({
		color: theme.palette.text.primary,
		display: "block",
	}),
	link: (theme) => ({
		color: "inherit",
		textDecoration: "none",
		display: "flex",
		alignItems: "center",
		gap: 4,

		"&:hover": {
			color: theme.palette.text.primary,
		},

		"& svg": {
			width: 12,
			height: 12,
		},
	}),
	shortDesc: {
		maxWidth: 280,
	},
} satisfies Record<string, Interpolation<Theme>>;
