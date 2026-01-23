import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import { CircleCheckBigIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

interface PaywallProps {
	message: string;
	description?: ReactNode;
	documentationLink?: string;
	documentationLinkText?: string;
	features?: Array<{
		text: string;
		link?: { href: string; text: string };
	}>;
	badgeText?: string;
	ctaText?: string;
	ctaLink?: string;
}

export const Paywall: FC<PaywallProps> = ({
	message,
	description,
	documentationLink,
	documentationLinkText = "Read the documentation",
	features,
	badgeText = "Premium",
	ctaText = "Learn about Premium",
	ctaLink = "https://coder.com/pricing#compare-plans",
}) => {
	const defaultFeatures: Array<{
		text: string;
		link?: { href: string; text: string };
	}> = [
		{ text: "High availability & workspace proxies" },
		{ text: "Multi-org & role-based access control" },
		{ text: "24x7 global support with SLA" },
		{ text: "Unlimited Git & external auth integrations" },
	];

	const displayFeatures = features ?? defaultFeatures;
	return (
		<div css={styles.root}>
			<div>
				<Stack direction="row" alignItems="center" className="mb-6">
					<h5 className="font-semibold font-inherit text-xl m-0">{message}</h5>
					<span
						css={[
							{
								fontSize: 10,
								height: 24,
								fontWeight: 600,
								textTransform: "uppercase",
								letterSpacing: "0.085em",
								padding: "0 12px",
								borderRadius: 9999,
								display: "flex",
								alignItems: "center",
								width: "fit-content",
								whiteSpace: "nowrap",
							},
							(theme) => ({
								backgroundColor: theme.branding.premium.background,
								border: `1px solid ${theme.branding.premium.border}`,
								color: theme.branding.premium.text,
							}),
						]}
					>
						{badgeText}
					</span>
				</Stack>

				{description && (
					<p className="font-inherit max-w-md text-sm mb-4">{description}</p>
				)}
				{documentationLink && (
					<Link
						href={documentationLink}
						target="_blank"
						rel="noreferrer"
						className="font-semibold"
					>
						{documentationLinkText}
					</Link>
				)}
			</div>
			<div className="w-px h-[220px] bg-highlight-purple/50 ml-2" />
			<Stack direction="column" alignItems="left" spacing={3}>
				<ul className="m-0 px-6 text-sm font-medium">
					{displayFeatures.map((feature, index) => (
						<li key={index} css={styles.feature}>
							<FeatureIcon />
							{feature.text}
							{feature.link && (
								<>
									{" "}
									<Link
										href={feature.link.href}
										target="_blank"
										rel="noreferrer"
										className="font-semibold text-sm"
									>
										{feature.link.text}
									</Link>
								</>
							)}
						</li>
					))}
				</ul>
				<div className="px-7">
					<Button asChild>
						<a href={ctaLink} target="_blank" rel="noreferrer">
							{ctaText}
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
