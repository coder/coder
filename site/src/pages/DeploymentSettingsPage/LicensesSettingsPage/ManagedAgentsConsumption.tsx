import type { Feature } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Link } from "components/Link/Link";
import dayjs from "dayjs";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
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
				<div className="flex flex-col gap-4 items-center justify-center">
					<div className="flex flex-col gap-2 items-center justify-center">
						<span className="text-base">Agent Workspace Builds Disabled</span>
						<span className="text-content-secondary text-center max-w-[464px] mt-2">
							Agent Workspace Builds are not included in your current license.
							Contact <Link href="mailto:sales@coder.com">sales</Link> to
							upgrade your license and unlock this feature.
						</span>
					</div>
				</div>
			</div>
		);
	}

	const usage = managedAgentFeature.actual;
	const included = managedAgentFeature.soft_limit;
	const limit = managedAgentFeature.limit;
	const startDate = managedAgentFeature.usage_period?.start;
	const endDate = managedAgentFeature.usage_period?.end;

	if (usage === undefined || usage < 0) {
		return <ErrorAlert error="Invalid usage data" />;
	}

	if (
		included === undefined ||
		included < 0 ||
		limit === undefined ||
		limit < 0
	) {
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

	const usagePercentage = Math.min((usage / limit) * 100, 100);
	const includedPercentage = Math.min((included / limit) * 100, 100);
	const remainingPercentage = Math.max(100 - includedPercentage, 0);

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium">Agent Workspace Builds</h3>

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
							Agent Workspace Builds are measured when you start an ephemeral
							workspace, purely for running an agentic workload. These are not
							to be confused with workspaces used for day-to-day development,
							even if AI tooling is involved.
						</p>
						<p>
							Today,{" "}
							<Link
								href={docs("/ai-coder/tasks")}
								target="_blank"
								rel="noreferrer"
							>
								Coder Tasks (via UI, CLI, or API)
							</Link>{" "}
							is the only way to create agentic workspaces, but additional
							protocols and APIs may be supported as standards emerge. Learn
							more in{" "}
							<Link
								href={docs("/ai-coder/ai-governance")}
								target="_blank"
								rel="noreferrer"
							>
								the Coder documentation
							</Link>
						</p>
						<ul>
							<li className="flex items-center gap-2">
								<div className="rounded-[2px] bg-highlight-green size-3 inline-block">
									<span className="sr-only">Legend for started workspaces</span>
								</div>
								Amount of started workspaces with an AI agent.
							</li>
							<li className="flex items-center gap-2">
								<div className="rounded-[2px] bg-content-disabled size-3 inline-block">
									<span className="sr-only">Legend for included allowance</span>
								</div>
								Included allowance from your current license plan.
							</li>
							<li className="flex items-center gap-2">
								<div className="size-3 inline-flex items-center justify-center">
									<span className="sr-only">
										Legend for total limit in the chart
									</span>
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
						className="absolute top-0 left-0 h-full bg-highlight-green transition-all duration-300"
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
