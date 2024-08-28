import KeyboardDoubleArrowRightIcon from "@mui/icons-material/KeyboardDoubleArrowRight";
import Link from "@mui/material/Link";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router-dom";

interface BreadcrumbsProps {
	children?: ReactNode;
}

export const Breadcrumbs: FC<BreadcrumbsProps> = ({ children }) => {
	return (
		<div css={{ display: "flex", flexDirection: "row", gap: 10, fontSize: 14 }}>
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
		<div
			css={{
				display: "flex",
				flexDirection: "row",
				gap: 10,
				alignItems: "center",
				fontWeight: 600,
				"&:first-of-type .chevron": {
					display: "none",
				},
			}}
		>
			<div className="chevron" css={{ display: "inline-block", paddingTop: 3 }}>
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

/* <Breadcrumbs>
	<Crumb href="/wibble">Wibble</Crumb>
	<Crumb href="/wibble/wobble">Wobble</Crumb>
</Breadcrumbs>;


<Breadcrumbs breadcrumbs={[
	{ href: "/wibble", children: "Wibble" },
	{ href: "/wibble/wobble", children: "Wobble" },
]} />; */
