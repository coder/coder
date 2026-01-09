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
						<span className="text-base">Managed AI Features Disabled</span>
						<span className="text-content-secondary text-center max-w-[464px] mt-2">
							Managed AI features are not included in your current license.
							Contact <MuiLink href="mailto:sales@coder.com">sales</MuiLink> to
							upgrade your license and unlock these features.
						</span>
					</Stack>
				</Stack>
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

	// Mock data for AI Users
	const mockAIUsersUsage = 20;
	const mockAIUsersIncluded = 50;
	const mockAIUsersUsagePercentage = Math.min(
		(mockAIUsersUsage / mockAIUsersIncluded) * 100,
		100,
	);

	// Mock group data
	const mockGroups = [
		{ name: "DevOps", using: 8, total: 14 },
		{ name: "Service Engineering", using: 7, total: 18 },
		{ name: "Platform Team", using: 3, total: 9 },
		{ name: "Backend", using: 2, total: 4 },
	];
	const mockFreeSeats = 5;

	// Mock data for Agentic Workspace Starts
	const mockWorkspacesUsage = 6347;
	const mockWorkspacesIncluded = 35000;
	const mockWorkspacesUsagePercentage = Math.min(
		(mockWorkspacesUsage / mockWorkspacesIncluded) * 100,
		100,
	);

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<header className="flex flex-col gap-2 items-start">
					<h3 className="text-md m-0 font-medium">Managed AI Usage</h3>
				</header>
			</div>

			<div className="p-6 border-0 border-t border-solid">
				<div className="space-y-8">
					{/* AI Users */}
					<div>
						<div className="mb-4">
							<h4 className="text-sm font-medium m-0 mb-2">
								Users with AI Governance Add-On
							</h4>
							<Collapsible>
								<CollapsibleTrigger asChild>
									<Button
										className={`
                      h-auto p-0 border-0 bg-transparent font-medium text-content-secondary text-xs
                      hover:bg-transparent hover:text-content-primary
                      [&[data-state=open]_svg]:rotate-90
                    `}
									>
										<ChevronRightIcon className="w-3 h-3" />
										Learn more
									</Button>
								</CollapsibleTrigger>

								<CollapsibleContent
									className={`
                    pt-2 pl-5 space-y-4 font-medium max-w-[720px]
                    text-xs text-content-secondary
                    [&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
                  `}
								>
									<p>
										Users entitled to use AI features like{" "}
										<MuiLink
											href={docs("/ai-coder/boundaries")}
											target="_blank"
											rel="noreferrer"
										>
											Bridge
										</MuiLink>
										,{" "}
										<MuiLink
											href={docs("/ai-coder/boundaries")}
											target="_blank"
											rel="noreferrer"
										>
											Boundaries
										</MuiLink>
										, or{" "}
										<MuiLink
											href={docs("/ai-coder/tasks")}
											target="_blank"
											rel="noreferrer"
										>
											Tasks
										</MuiLink>
										.
									</p>
								</CollapsibleContent>
							</Collapsible>
						</div>

						<div className="flex justify-between text-sm text-content-secondary mb-4">
							<span>
								{startDate ? dayjs(startDate).format("MMMM D, YYYY") : ""}
							</span>
							<span>
								{endDate ? dayjs(endDate).format("MMMM D, YYYY") : ""}
							</span>
						</div>

						<div className="relative h-6 bg-surface-secondary rounded overflow-hidden">
							<div
								className="absolute top-0 left-0 h-full bg-highlight-green transition-all duration-300"
								style={{ width: `${mockAIUsersUsagePercentage}%` }}
							/>
						</div>

						<div className="flex justify-between mt-4 text-sm">
							<div className="flex flex-col items-start">
								<span className="text-content-secondary">Actual:</span>
								<span className="font-medium">
									{mockAIUsersUsage.toLocaleString()}
								</span>
							</div>
							<div className="flex flex-col items-end">
								<span className="text-content-secondary">Included:</span>
								<span className="font-medium">
									{mockAIUsersIncluded.toLocaleString()}
								</span>
							</div>
						</div>

						{/* Group breakdown */}
						<div className="mt-6 space-y-3">
							<div className="flex items-center justify-between">
								<h5 className="text-xs font-medium text-content-secondary uppercase tracking-wider m-0">
									By Group
								</h5>
								<MuiLink
									href="#"
									className="text-xs"
								>
									Manage Group Entitlements
								</MuiLink>
							</div>
							<div className="space-y-2">
								{mockGroups.map((group) => (
									<div
										key={group.name}
										className="flex items-center justify-between p-3 bg-surface-secondary rounded"
									>
										<div className="flex-1">
											<div className="text-sm font-medium">{group.name}</div>
											<div className="text-xs text-content-secondary mt-1">
												{group.using} using / {group.total} total
											</div>
										</div>
									</div>
								))}
								<div className="flex items-center justify-between p-3 border border-dashed border-highlight-green rounded">
									<div className="flex-1">
										<div className="text-sm font-medium">Free Seats</div>
										<div className="text-xs text-content-secondary mt-1">
											{mockFreeSeats} available
										</div>
									</div>
								</div>
							</div>
						</div>
					</div>

					{/* Agentic Workspace Starts */}
					<div>
						<div className="mb-4">
							<h4 className="text-sm font-medium m-0 mb-2">
								Agentic Workspace Starts
							</h4>
							<Collapsible>
								<CollapsibleTrigger asChild>
									<Button
										className={`
                      h-auto p-0 border-0 bg-transparent font-medium text-content-secondary text-xs
                      hover:bg-transparent hover:text-content-primary
                      [&[data-state=open]_svg]:rotate-90
                    `}
									>
										<ChevronRightIcon className="w-3 h-3" />
										Learn more
									</Button>
								</CollapsibleTrigger>

								<CollapsibleContent
									className={`
                    pt-2 pl-5 space-y-4 font-medium max-w-[720px]
                    text-xs text-content-secondary
                    [&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
                  `}
								>
									<p>
										Agentic Workspace Starts are measured when you start an
										emphemeral workspace, purely for running an agentic
										workload. These are not to be confused with workspaces used
										for day-to-day development, even if AI tooling is involved.
									</p>
									<p>
										Today,{" "}
										<MuiLink
											href={docs("/ai-coder/tasks")}
											target="_blank"
											rel="noreferrer"
										>
											Coder Tasks (via UI, CLI, or API)
										</MuiLink>{" "}
										is the only way to create agentic workspaces, but additional
										protocols and APIs may be supported as standards emerge.
									</p>
								</CollapsibleContent>
							</Collapsible>
						</div>

						<div className="flex justify-between text-sm text-content-secondary mb-4">
							<span>
								{startDate ? dayjs(startDate).format("MMMM D, YYYY") : ""}
							</span>
							<span>
								{endDate ? dayjs(endDate).format("MMMM D, YYYY") : ""}
							</span>
						</div>

						<div className="relative h-6 bg-surface-secondary rounded overflow-hidden">
							<div
								className="absolute top-0 left-0 h-full bg-highlight-green transition-all duration-300"
								style={{ width: `${mockWorkspacesUsagePercentage}%` }}
							/>
						</div>

						<div className="flex justify-between mt-4 text-sm">
							<div className="flex flex-col items-start">
								<span className="text-content-secondary">Actual:</span>
								<span className="font-medium">
									{mockWorkspacesUsage.toLocaleString()}
								</span>
							</div>
							<div className="flex flex-col items-end">
								<span className="text-content-secondary">Included:</span>
								<span className="font-medium">
									{mockWorkspacesIncluded.toLocaleString()}
								</span>
							</div>
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};
