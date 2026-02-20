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
import { cn } from "utils/cn";
import { docs } from "utils/docs";

interface ManagedAgentsConsumptionProps {
	managedAgentFeature?: Feature;
}

export const ManagedAgentsConsumption: FC<ManagedAgentsConsumptionProps> = ({
	managedAgentFeature,
}) => {
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

	const usage = managedAgentFeature.actual;
	const included = managedAgentFeature.soft_limit;
	const startDate = managedAgentFeature.usage_period?.start;
	const endDate = managedAgentFeature.usage_period?.end;

	if (usage === undefined || usage < 0) {
		return <ErrorAlert error="Invalid usage data" />;
	}

	if (included === undefined || included < 0) {
		return <ErrorAlert error="Invalid license usage limits" />;
	}

	if (!startDate || !endDate) {
		return <ErrorAlert error="Missing license usage period" />;
	}

	const start = dayjs(startDate);
	const end = dayjs(endDate);
	if (!start.isValid() || !end.isValid() || !start.isBefore(end)) {
		return <ErrorAlert error="Invalid license usage period" />;
	}

	const usagePercentage = Math.min((usage / included) * 100, 100);

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium">Managed AI Agents Usage</h3>

						<CollapsibleTrigger asChild>
							<Button
								className={`
                  h-auto p-0 border-0 bg-transparent font-medium text-content-secondary
                  hover:bg-transparent hover:text-content-primary
                  [&[data-state=open]_svg]:rotate-90
                `}
							>
								<ChevronRightIcon />
								Learn more
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
							<MuiLink
								href={docs("/ai-coder/tasks")}
								target="_blank"
								rel="noreferrer"
							>
								Coder Tasks
							</MuiLink>{" "}
							and upcoming managed AI features are included in Coder Premium
							licenses during beta. Usage limits and pricing subject to change.
						</p>
						<ul>
							<li className="flex items-center gap-2">
								<div className="rounded-[2px] bg-highlight-green size-3 inline-block">
									<span className="sr-only">Legend for started workspaces</span>
								</div>
								Amount of started workspaces with an AI agent.
							</li>
							<li className="flex items-center gap-2">
								<div className="rounded-[2px] bg-highlight-orange size-3 inline-block">
									<span className="sr-only">
										Legend for usage exceeding included allowance
									</span>
								</div>
								Usage has exceeded included allowance from your current license
								plan.
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
						className={cn(
							"absolute top-0 left-0 h-full transition-all duration-300",
							usagePercentage < 100
								? "bg-highlight-green"
								: "bg-highlight-orange",
						)}
						style={{ width: `${usagePercentage}%` }}
					/>
				</div>

				<div className="relative hidden lg:flex justify-between mt-4 text-sm">
					<div className="flex flex-col items-start">
						<span className="text-content-secondary">Actual:</span>
						<span className="font-medium">{usage.toLocaleString()}</span>
					</div>

					<div className="flex flex-col items-end">
						<span className="text-content-secondary">Included:</span>
						<span className="font-medium">{included.toLocaleString()}</span>
					</div>
				</div>

				<div className="flex lg:hidden flex-col gap-3 mt-4 text-sm">
					<div className="flex justify-between">
						<div className="flex flex-col items-start">
							<span className="text-content-secondary">Actual:</span>
							<span className="font-medium">{usage.toLocaleString()}</span>
						</div>
						<div className="flex flex-col items-end">
							<span className="text-content-secondary">Included:</span>
							<span className="font-medium">{included.toLocaleString()}</span>
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};
