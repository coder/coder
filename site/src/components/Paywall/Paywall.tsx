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
		<div className={classNames.root}>
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
					<li className={classNames.feature}>
						<FeatureIcon />
						High availability & workspace proxies
					</li>
					<li className={classNames.feature}>
						<FeatureIcon />
						Multi-org & role-based access control
					</li>
					<li className={classNames.feature}>
						<FeatureIcon />
						24x7 global support with SLA
					</li>
					<li className={classNames.feature}>
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
			className="size-icon-sm text-violet-400"
		/>
	);
};

const classNames = {
	root: [
		"flex flex-row justify-center items-center min-h-[280px] p-6 rounded-lg gap-8",
		"bg-gradient-to-b from-transparent to-violet-950",
	].join(" "),
	feature: "flex items-center p-[3px] gap-2",
};
