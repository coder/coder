import type { Interpolation, Theme } from "@emotion/react";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import { PremiumBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";
import { docs } from "utils/docs";

export interface PaywallProps {
	message: string;
	description?: ReactNode;
	documentationLink?: string;
}

export const Paywall: FC<PaywallProps> = ({
	message,
	description,
	documentationLink,
}) => {
	return (
		<div css={styles.root}>
			<div>
				<Stack direction="row" alignItems="center" css={{ marginBottom: 24 }}>
					<h5 css={styles.title}>{message}</h5>
					<PremiumBadge />
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
			<Stack direction="column" alignItems="left" spacing={3}>
				<ul css={styles.featureList}>
					<li css={styles.feature}>
						<FeatureIcon />
						High availability & workspace proxies
					</li>
					<li css={styles.feature}>
						<FeatureIcon />
						Multi-org & role-based access control
					</li>
					<li css={styles.feature}>
						<FeatureIcon />
						24x7 global support with SLA
					</li>
					<li css={styles.feature}>
						<FeatureIcon />
						Unlimited Git & external auth integrations
					</li>
				</ul>
				<div css={styles.learnButton}>
					<Button
						href={docs("/licensing")}
						target="_blank"
						rel="noreferrer"
						startIcon={<span css={{ fontSize: 22 }}>&rarr;</span>}
						variant="outlined"
						color="neutral"
					>
						Learn about Premium
					</Button>
				</div>
			</Stack>
		</div>
	);
};

const FeatureIcon: FC = () => {
	return (
		<TaskAltIcon
			css={[
				(theme) => ({
					color: theme.branding.premium.border,
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
		backgroundImage: `linear-gradient(160deg, transparent, ${theme.branding.premium.background})`,
		border: `1px solid ${theme.branding.premium.border}`,
	}),
	title: {
		fontWeight: 600,
		fontFamily: "inherit",
		fontSize: 22,
		margin: 0,
	},
	description: () => ({
		fontFamily: "inherit",
		maxWidth: 460,
		fontSize: 14,
	}),
	separator: (theme) => ({
		width: 1,
		height: 220,
		backgroundColor: theme.branding.premium.divider,
		marginLeft: 8,
	}),
	learnButton: {
		padding: "0 28px",
	},
	featureList: {
		listStyle: "none",
		margin: 0,
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
