import type { Interpolation, Theme } from "@emotion/react";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import { EnterpriseBadge, PremiumBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";
import theme from "theme";
import { docs } from "utils/docs";

export interface PaywallProps {
	type: "enterprise" | "premium";
	message: string;
	description?: ReactNode;
	documentationLink?: string;
}

export const Paywall: FC<PaywallProps> = ({
	type,
	message,
	description,
	documentationLink,
}) => {
	return (
		<div
			css={[
				styles.root,
				(theme) => ({
					backgroundImage: `linear-gradient(160deg, transparent, ${theme.branding.paywall[type].background})`,
					border: `1px solid ${theme.branding.paywall[type].border}`,
				}),
			]}
		>
			<div>
				<Stack direction="row" alignItems="center" css={{ marginBottom: 24 }}>
					<h5 css={styles.title}>{message}</h5>
					{type === "enterprise" ? <EnterpriseBadge /> : <PremiumBadge />}
				</Stack>

				{description && <p css={styles.description}>{description}</p>}
				<Link
					href={documentationLink}
					target="_blank"
					rel="noreferrer"
					css={{ fontWeight: 600 }}
				>
					Read the documentation
				</Link>
			</div>
			<div css={styles.separator} />
			<Stack direction="column" alignItems="center" spacing={3}>
				<ul css={styles.featureList}>
					<li css={styles.feature}>
						<FeatureIcon type={type} />
						{type === "premium"
							? "High availability & workspace proxies"
							: "Template access control"}
					</li>
					<li css={styles.feature}>
						<FeatureIcon type={type} />
						{type === "premium"
							? "Multi-org & role-based access control"
							: "User groups"}
					</li>
					<li css={styles.feature}>
						<FeatureIcon type={type} />
						{type === "premium"
							? "24x7 global support with SLA"
							: "24 hour support"}
					</li>
					<li css={styles.feature}>
						<FeatureIcon type={type} />
						{type === "premium"
							? "Unlimited Git & external auth integrations"
							: "Audit logs"}
					</li>
				</ul>
				<Button
					href={docs("/enterprise")}
					target="_blank"
					rel="noreferrer"
					startIcon={<span css={{ fontSize: 22 }}>&rarr;</span>}
					variant="outlined"
					color="neutral"
				>
					Learn about {type === "enterprise" ? "Enterprise" : "Premium"}
				</Button>
			</Stack>
		</div>
	);
};

export interface FeatureIconProps {
	type: "enterprise" | "premium";
}

const FeatureIcon: FC<FeatureIconProps> = ({ type }) => {
	return (
		<TaskAltIcon
			css={[
				(theme) => ({
					color: theme.branding.paywall[type].border,
				}),
			]}
		/>
	);
};

const styles = {
	root: (theme) => ({
		display: "flex",
		flexDirection: "row",
		justifyContent: "center",
		alignItems: "center",
		minHeight: 280,
		padding: 24,
		borderRadius: 8,
		gap: 32,
	}),
	title: {
		fontWeight: 600,
		fontFamily: "inherit",
		fontSize: 22,
		margin: 0,
	},
	description: (theme) => ({
		fontFamily: "inherit",
		maxWidth: 460,
		fontSize: 14,
	}),
	separator: (theme) => ({
		width: 1,
		height: 220,
		backgroundColor: theme.branding.paywall.premium.divider,
		marginLeft: 8,
	}),
	featureList: {
		listStyle: "none",
		margin: 0,
		marginRight: 8,
		padding: "0 24px",
		fontSize: 14,
		fontWeight: 500,
	},
	featureIcon: (theme) => ({
		color: theme.roles.active.fill.outline,
	}),
	feature: {
		display: "flex",
		alignItems: "center",
		padding: 3,
		gap: 8,
	},
} satisfies Record<string, Interpolation<Theme>>;
