import type { Interpolation, Theme } from "@emotion/react";
import KeyboardDoubleArrowRightIcon from "@mui/icons-material/KeyboardDoubleArrowRight";
import Link from "@mui/material/Link";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router-dom";

interface BreadcrumbsProps {
	children?: ReactNode;
	className?: string;
}

export const Breadcrumbs: FC<BreadcrumbsProps> = ({ children, className }) => {
	return (
		<div
			className={className}
			css={{
				display: "flex",
				alignItems: "center",
				flexDirection: "row",
				gap: 10,
				fontSize: 14,
			}}
		>
			{children}
		</div>
	);
};

interface CrumbProps {
	active?: boolean;
	href?: string;
	children?: ReactNode;
}

export const Crumb: FC<CrumbProps> = ({ active, href, children }) => {
	return (
		<div css={styles.crumb}>
			<div className="chevron">
				<KeyboardDoubleArrowRightIcon css={{ width: 14, height: 14 }} />
			</div>
			{href && !active ? (
				<Link component={RouterLink} to={href} css={{ color: "inherit" }}>
					{children}
				</Link>
			) : (
				<span>{children}</span>
			)}
		</div>
	);
};

const styles = {
	crumb: {
		display: "flex",
		alignItems: "center",
		flexDirection: "row",
		gap: 10,
		fontWeight: 600,

		"&:first-of-type .chevron": {
			display: "none",
		},

		"& .chevron": {
			display: "inline-block",
			paddingTop: 3,
			height: "fit-content",
		},
	},
} satisfies Record<string, Interpolation<Theme>>;
