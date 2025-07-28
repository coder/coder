import MuiLink from "@mui/material/Link";
import type { Feature } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";

interface ManagedAgentsConsumptionProps {
	managedAgentFeature?: Feature;
}

const validateFeature = (feature?: Feature): string | null => {
	if (!feature) {
		return null; // No feature is valid (will show disabled state)
	}

	// If enabled, we need valid numeric data
	if (feature.enabled) {
		if (
			feature.actual === undefined ||
			feature.soft_limit === undefined ||
			feature.limit === undefined
		) {
			return "Managed agent feature is enabled but missing required usage data (actual, soft_limit, or limit).";
		}

		if (feature.actual < 0 || feature.soft_limit < 0 || feature.limit < 0) {
			return "Managed agent feature contains invalid negative values for usage metrics.";
		}

		if (feature.soft_limit > feature.limit) {
			return "Managed agent feature has invalid configuration: soft limit exceeds total limit.";
		}

		// Validate usage period if present
		if (feature.usage_period) {
			const start = dayjs(feature.usage_period.start);
			const end = dayjs(feature.usage_period.end);

			if (!start.isValid() || !end.isValid()) {
				return "Managed agent feature has invalid usage period dates.";
			}

			if (end.isBefore(start)) {
				return "Managed agent feature has invalid usage period: end date is before start date.";
			}
		}
	}

	return null; // Valid
};

export const ManagedAgentsConsumption: FC<ManagedAgentsConsumptionProps> = ({
	managedAgentFeature,
}) => {
	// Validate the feature data
	const validationError = validateFeature(managedAgentFeature);
	if (validationError) {
		return <ErrorAlert error={new Error(validationError)} />;
	}

	// If no feature is provided or it's disabled, show disabled state
	if (!managedAgentFeature?.enabled) {
		return (
			<div className="min-h-60 flex items-center justify-center rounded-lg border border-solid p-12">
				<Stack alignItems="center" spacing={1}>
					<Stack alignItems="center" spacing={0.5}>
						<span className="text-base">Managed AI Agents Disabled</span>
						<span className="text-content-secondary text-center max-w-[464px] mt-2">
							Managed AI agents are not included in your current license.
							Contact <MuiLink href="mailto:sales@coder.com">sales</MuiLink> to
							upgrade your license and unlock this feature.
						</span>
					</Stack>
				</Stack>
			</div>
		);
	}

	const usage = managedAgentFeature.actual || 0;
	const included = managedAgentFeature.soft_limit || 0;
	const limit = managedAgentFeature.limit || 0;
	const startDate = managedAgentFeature.usage_period?.start || "";
	const endDate = managedAgentFeature.usage_period?.end || "";

	const usagePercentage = Math.min((usage / limit) * 100, 100);
	const includedPercentage = Math.min((included / limit) * 100, 100);
	const remainingPercentage = Math.max(100 - includedPercentage, 0);

	// Determine usage bar color based on percentage
	const getUsageColor = () => {
		const actualUsagePercent = (usage / limit) * 100;
		if (actualUsagePercent >= 100) {
			return "bg-highlight-red"; // Critical: at or over limit
		}
		if (actualUsagePercent >= 80) {
			return "bg-surface-orange"; // Warning: approaching limit
		}
		return "bg-highlight-green"; // Normal: safe usage
	};

	const usageBarColor = getUsageColor();

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium">
							Managed agents consumption
						</h3>

						<CollapsibleTrigger asChild>
							<Button
								className={`
                  h-auto p-0 border-0 bg-transparent font-medium text-content-secondary
                  hover:bg-transparent hover:text-content-primary
                  [&[data-state=open]_svg]:rotate-90
                `}
							>
								<ChevronRightIcon />
								How we calculate managed agents consumption
							</Button>
						</CollapsibleTrigger>
					</header>

					<CollapsibleContent
						className={`
              pt-2 pl-7 pr-5 space-y-4 font-medium max-w-[720px]
              text-sm text-content-secondary
              [&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
            `}
					>
						<p>
							Managed agents are counted based on the amount of successfully
							started workspaces with an AI agent.
						</p>
						<ul>
							<li className="flex items-center gap-2">
								<div
									className={`rounded-[2px] ${usageBarColor} size-3 inline-block`}
									aria-label="Legend for current usage in the chart"
								/>
								Amount of started workspaces with an AI agent.
							</li>
							<li className="flex items-center gap-2">
								<div
									className="rounded-[2px] bg-content-disabled size-3 inline-block"
									aria-label="Legend for included allowance in the chart"
								/>
								Included allowance from your current license plan.
							</li>
							<li className="flex items-center gap-2">
								<div
									className="size-3 inline-flex items-center justify-center"
									aria-label="Legend for total limit in the chart"
								>
									<div className="w-full border-b-1 border-t-1 border-dashed border-content-disabled" />
								</div>
								Total limit after which further AI workspace builds will be
								blocked.
							</li>
						</ul>
					</CollapsibleContent>
				</Collapsible>
			</div>

			<div className="p-6 border-0 border-t border-solid">
				<div className="flex justify-between text-sm text-content-secondary mb-4">
					<span>
						{startDate ? dayjs(startDate).format("MMMM D, YYYY") : ""}
					</span>
					<span>{endDate ? dayjs(endDate).format("MMMM D, YYYY") : ""}</span>
				</div>

				<div className="relative h-6 bg-surface-secondary rounded overflow-hidden">
					<div
						className={`absolute top-0 left-0 h-full ${usageBarColor} transition-all duration-300`}
						style={{ width: `${usagePercentage}%` }}
					/>

					<div
						className="absolute top-0 h-full bg-content-disabled opacity-30"
						style={{
							left: `${includedPercentage}%`,
							width: `${remainingPercentage}%`,
						}}
					/>
				</div>

				<div className="relative hidden lg:flex justify-between mt-4 text-sm">
					<div className="flex flex-col items-start">
						<span className="text-content-secondary">Actual:</span>
						<span className="font-medium">{usage.toLocaleString()}</span>
					</div>

					<div
						className="absolute flex flex-col items-center transform -translate-x-1/2"
						style={{
							left: `${Math.max(Math.min(includedPercentage, 90), 10)}%`,
						}}
					>
						<span className="text-content-secondary">Included:</span>
						<span className="font-medium">{included.toLocaleString()}</span>
					</div>

					<div className="flex flex-col items-end">
						<span className="text-content-secondary">Limit:</span>
						<span className="font-medium">{limit.toLocaleString()}</span>
					</div>
				</div>

				<div className="flex lg:hidden flex-col gap-3 mt-4 text-sm">
					<div className="flex justify-between">
						<div className="flex flex-col items-start">
							<span className="text-content-secondary">Actual:</span>
							<span className="font-medium">{usage.toLocaleString()}</span>
						</div>
						<div className="flex flex-col items-center">
							<span className="text-content-secondary">Included:</span>
							<span className="font-medium">{included.toLocaleString()}</span>
						</div>
						<div className="flex flex-col items-end">
							<span className="text-content-secondary">Limit:</span>
							<span className="font-medium">{limit.toLocaleString()}</span>
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};
