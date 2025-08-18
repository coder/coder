import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import { PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import { CircleCheckBigIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

interface PaywallProps {
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
				<Stack direction="row" alignItems="center" className="mb-6">
					<h5 className="font-semibold font-inherit text-xl m-0">{message}</h5>
					<PremiumBadge />
				</Stack>

				{description && (
					<p className="font-inherit max-w-md text-sm">{description}</p>
				)}
				<Link
					href={documentationLink}
					target="_blank"
					rel="noreferrer"
					className="font-semibold"
				>
					Read the documentation
				</Link>
			</div>
			<div className="w-px h-[220px] bg-highlight-purple/50 ml-2" />
			<Stack direction="column" alignItems="left" spacing={3}>
				<ul className="m-0 px-6 text-sm font-medium">
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
				<div className="px-7">
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
		<CircleCheckBigIcon
			aria-hidden="true"
			className="size-icon-sm"
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
	feature: {
		display: "flex",
		alignItems: "center",
		padding: 3,
		gap: 8,
	},
} satisfies Record<string, Interpolation<Theme>>;
