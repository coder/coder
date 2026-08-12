import { InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Separator } from "#/components/Separator/Separator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

// Sentinel allocation claim value meaning the license grants unlimited
// agent runtime hours (AgentRuntimeHoursUnlimitedAllocation in
// enterprise/coderd/license).
const unlimitedAllocation = -1;

// Mirrors the backend's maxConcurrentRootAgents constant, which caps
// concurrent chats once the hard limit is reached. It is not exposed via
// the API, so keep this value in sync with the backend.
const maxConcurrentChatsOverHardLimit = 5;

type CoderAgentsProductCardProps = {
	/**
	 * The license's agent_runtime_hours_allocation claim, in hours.
	 * Undefined or non-positive (other than the -1 unlimited sentinel)
	 * means the license does not include Coder Agents hours.
	 */
	allocation?: number;
	/**
	 * Agent runtime hours used in the current usage period, from the
	 * merged entitlements. Undefined when usage does not apply to this
	 * license (another license provides the feature) or is unknown.
	 */
	actual?: number;
	/** Usage is above this license's allocation. */
	isExceeded: boolean;
	/** Usage is at or above this license's hard limit. */
	isHardLimitExceeded: boolean;
};

const MetricLabel: FC<{ label: string; tooltip: string }> = ({
	label,
	tooltip,
}) => (
	<div className="flex items-center gap-1 font-medium text-content-secondary">
		<span>{label}</span>
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={`${label} information`}
					className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
				>
					<InfoIcon className="size-3" />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs">
				{tooltip}
			</TooltipContent>
		</Tooltip>
	</div>
);

const CardContainer: FC<{ className?: string; children: ReactNode }> = ({
	className,
	children,
}) => (
	<div
		className={cn(
			"min-w-[320px] flex-1 rounded-sm border px-6 py-4",
			className,
		)}
	>
		<div className="text-sm font-medium text-content-primary">Coder Agents</div>
		{children}
	</div>
);

// TODO: placeholder tooltip copy pending product review.
const totalAgentHoursTooltip =
	"Total agent runtime hours used out of the hours included in this license.";
const concurrentChatsTooltip =
	"Number of Coder Agents chats that can run at the same time.";

export const CoderAgentsProductCard: FC<CoderAgentsProductCardProps> = ({
	allocation,
	actual,
	isExceeded,
	isHardLimitExceeded,
}) => {
	const isUnlimited = allocation === unlimitedAllocation;
	const grantsAgentHours =
		allocation !== undefined && (allocation > 0 || isUnlimited);

	if (!grantsAgentHours) {
		return (
			<CardContainer className="border-dashed border-highlight-purple">
				<div className="mt-3 text-xs">
					<MetricLabel
						label="Max concurrent chats"
						tooltip={concurrentChatsTooltip}
					/>
					<div className="mt-0.5 font-normal text-content-primary">
						{maxConcurrentChatsOverHardLimit}
					</div>
					{actual !== undefined && (
						<div className="mt-3 font-medium text-content-secondary">
							Agent hours used:{" "}
							<span className="font-normal text-content-primary">
								{actual.toLocaleString("en-US")}
							</span>
						</div>
					)}
				</div>
				<Button asChild className="mt-4 w-full">
					<a href="mailto:sales@coder.com">Upgrade</a>
				</Button>
			</CardContainer>
		);
	}

	const isOverage = isExceeded || isHardLimitExceeded;
	const actualLabel =
		actual === undefined ? "\u2014" : actual.toLocaleString("en-US");

	return (
		<CardContainer
			className={cn(
				"border-solid",
				isOverage ? "border-border-destructive" : "border-border",
			)}
		>
			<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
				<div>
					<MetricLabel
						label="Total Agent hours"
						tooltip={totalAgentHoursTooltip}
					/>
					<div className="mt-0.5 font-normal text-content-primary">
						{isUnlimited ? (
							"Unlimited"
						) : (
							<>
								<span className={cn({ "text-content-destructive": isOverage })}>
									{actualLabel}
								</span>{" "}
								/ {allocation.toLocaleString("en-US")}
							</>
						)}
					</div>
				</div>
				<div>
					<MetricLabel
						label="Concurrent chats"
						tooltip={concurrentChatsTooltip}
					/>
					<div className="mt-0.5 font-normal text-content-primary">
						{isHardLimitExceeded
							? maxConcurrentChatsOverHardLimit
							: "Unlimited"}
					</div>
				</div>
			</div>
			<div className="mt-4 flex items-center gap-2 text-xs">
				<Link asChild size="sm" showExternalIcon={false}>
					<RouterLink to="/deployment/groups">Manage usage</RouterLink>
				</Link>
				<Separator orientation="vertical" className="h-3" />
				<Link asChild size="sm" showExternalIcon={false}>
					<RouterLink to="/ai/settings/coder-agents">Agent settings</RouterLink>
				</Link>
			</div>
		</CardContainer>
	);
};
