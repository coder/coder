import type { Interpolation, Theme } from "@emotion/react";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import Link from "@mui/material/Link";
import { PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";

export interface PopoverPaywallProps {
	message: string;
	description?: ReactNode;
	documentationLink?: string;
}

export const PopoverPaywall: FC<PopoverPaywallProps> = ({
	message,
	description,
	documentationLink,
}) => {
	return (
		<div
			css={[
				styles.root,
				(theme) => ({
					backgroundImage: `linear-gradient(160deg, transparent, ${theme.branding.premium.background})`,
					border: `1px solid ${theme.branding.premium.border}`,
				}),
			]}
		>
			<div>
				<Stack direction="row" alignItems="center" css={{ marginBottom: 18 }}>
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
			<Stack direction="column" alignItems="left" spacing={2}>
				<ul css={styles.featureList}>
					<li css={styles.feature}>
						<FeatureIcon /> High availability & workspace proxies
					</li>
					<li css={styles.feature}>
						<FeatureIcon /> Multi-org & role-based access control
					</li>
					<li css={styles.feature}>
						<FeatureIcon /> 24x7 global support with SLA
					</li>
					<li css={styles.feature}>
						<FeatureIcon /> Unlimited Git & external auth integrations
					</li>
				</ul>
				<div css={styles.learnButton}>
					<Button asChild>
						<a
							href="https://coder.com/pricing#compare-plans"
							target="_blank"
							rel="noreferrer"
						>
							Learn about Premium
						</a>
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
		alignItems: "center",
		maxWidth: 770,
		padding: "24px 36px",
		borderRadius: 8,
		gap: 18,
	}),
	title: {
		fontWeight: 600,
		fontFamily: "inherit",
		fontSize: 18,
		margin: 0,
	},
	description: (theme) => ({
		marginTop: 8,
		fontFamily: "inherit",
		maxWidth: 360,
		lineHeight: "160%",
		color: theme.palette.text.secondary,
		fontSize: 14,
	}),
	separator: (theme) => ({
		width: 1,
		height: 180,
		backgroundColor: theme.palette.divider,
		marginLeft: 8,
	}),
	featureList: {
		listStyle: "none",
		margin: 0,
		marginRight: 8,
		padding: "0 0 0 24px",
		fontSize: 13,
		fontWeight: 500,
	},
	learnButton: {
		padding: "0 28px",
	},
	feature: {
		display: "flex",
		alignItems: "center",
		padding: 3,
		gap: 8,
		lineHeight: 1.2,
	},
} satisfies Record<string, Interpolation<Theme>>;
