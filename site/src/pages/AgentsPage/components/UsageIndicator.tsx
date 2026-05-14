import dayjs from "dayjs";
import { InfoIcon } from "lucide-react";
import { type FC, Fragment, type ReactNode } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import { chatUsageLimitStatus } from "#/api/queries/chats";
import { workspaceQuota } from "#/api/queries/workspaceQuota";
import { workspaces } from "#/api/queries/workspaces";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { cn } from "#/utils/cn";
import { formatCostMicros } from "#/utils/currency";
import { getUsageLimitPeriodLabel } from "./ChatCostSummaryView";

type UsageSeverity = "normal" | "warning" | "exceeded";

type UsageSectionData = {
	id: string;
	title: string;
	progressLabel: string;
	percent: number;
	detail: ReactNode;
	secondaryDetail?: ReactNode;
	tooltip?: ReactNode;
	severity?: UsageSeverity;
};

const numberFormatter = new Intl.NumberFormat("en-US");

export const UsageIndicator: FC = () => {
	const { data: chatUsage, isError: isChatUsageError } = useQuery(
		chatUsageLimitStatus(),
	);
	const { user } = useAuthenticated();
	const { organizations } = useDashboard();
	const organizationName =
		organizations.find((org) => org.is_default)?.name ?? "";
	const username = user.username;
	const { data: quota, isError: isQuotaError } = useQuery({
		...workspaceQuota(organizationName, username),
		enabled: organizationName !== "" && username !== "",
	});
	const hasWorkspaceQuotaUsage =
		quota !== undefined && quota.budget >= 0 && quota.credits_consumed > 0;
	const workspacesQuery = useQuery({
		...workspaces({
			q: `owner:me organization:${organizationName}`,
			limit: 0,
		}),
		enabled: hasWorkspaceQuotaUsage && organizationName !== "",
	});
	const sections: UsageSectionData[] = [];

	if (!isChatUsageError && chatUsage?.is_limited) {
		const spendLimit = chatUsage.spend_limit_micros ?? 0;
		const currentSpend = chatUsage.current_spend;
		const periodLabel = getUsageLimitPeriodLabel(chatUsage.period);
		const exceeded = spendLimit > 0 && currentSpend >= spendLimit;

		sections.push({
			id: "ai-usage",
			title: `${periodLabel} Usage`,
			progressLabel: `${periodLabel} spend usage`,
			percent: getPercent(currentSpend, spendLimit),
			severity: getSeverity(currentSpend, spendLimit),
			detail: (
				<>
					{formatCostMicros(currentSpend)} of {formatCostMicros(spendLimit)}{" "}
					used
					{exceeded && (
						<span className="ml-1 text-content-destructive">
							(limit exceeded)
						</span>
					)}
				</>
			),
			secondaryDetail: chatUsage.period_end
				? `Resets ${dayjs(chatUsage.period_end).format("MMM D, YYYY")}`
				: undefined,
		});
	}

	if (!isQuotaError && hasWorkspaceQuotaUsage) {
		const creditsConsumed = quota.credits_consumed;
		const workspaceCount = workspacesQuery.isError
			? undefined
			: getWorkspaceCount(workspacesQuery.data?.count);
		const quotaDetail =
			workspaceCount === undefined
				? `${formatNumber(creditsConsumed)} of ${formatNumber(quota.budget)} credits used`
				: `${formatNumber(workspaceCount)} ${workspaceCount === 1 ? "workspace" : "workspaces"} using ${formatNumber(creditsConsumed)} of ${formatNumber(quota.budget)} credits`;

		sections.push({
			id: "workspace-quota",
			title: "Workspace quota",
			progressLabel: "Workspace quota usage",
			percent: getPercent(creditsConsumed, quota.budget),
			severity: getSeverity(creditsConsumed, quota.budget),
			detail: quotaDetail,
			tooltip:
				"Workspaces, stopped or running, may consume credits. Stop or delete unused ones to free quota.",
		});
	}

	if (sections.length === 0) {
		return null;
	}

	return <UsageMenu sections={sections} />;
};

const UsageMenu: FC<{ sections: readonly UsageSectionData[] }> = ({
	sections,
}) => {
	const triggerLabel =
		sections.length > 1 ? "Usage" : (sections[0]?.title ?? "Usage");

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="ml-auto flex self-stretch flex-col items-center justify-center gap-1 border-none bg-transparent px-3 cursor-pointer select-none transition-colors text-content-secondary hover:bg-surface-tertiary/50 outline-none text-[13px]"
				>
					<span className="shrink-0 whitespace-nowrap text-center">
						{triggerLabel}
					</span>
					<UsageTriggerProgress sections={sections} />
				</button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="min-w-auto w-[240px]">
				{sections.map((section, index) => (
					<Fragment key={section.id}>
						{index > 0 && <DropdownMenuSeparator />}
						<UsageSection section={section} />
					</Fragment>
				))}

				<DropdownMenuSeparator />

				<DropdownMenuItem asChild>
					<Link to="/agents/analytics">View usage</Link>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const UsageTriggerProgress: FC<{ sections: readonly UsageSectionData[] }> = ({
	sections,
}) => {
	const size = sections.length > 1 ? "compact" : "default";

	return (
		<div className="flex w-24 shrink-0 flex-col gap-0.5">
			{sections.map((section) => (
				<UsageProgress
					key={section.id}
					ariaLabel={section.progressLabel}
					percent={section.percent}
					severity={section.severity}
					size={size}
					className="w-full"
				/>
			))}
		</div>
	);
};

const UsageSection: FC<{ section: UsageSectionData }> = ({ section }) => {
	const roundedPercent = Math.round(section.percent);

	return (
		<>
			<div className="flex items-center justify-between gap-2 px-2 py-1.5">
				<span className="truncate text-sm font-medium text-content-primary">
					{section.title}
				</span>
				<span
					className={cn("shrink-0 text-xs", getTextClassName(section.severity))}
				>
					{roundedPercent}%
				</span>
			</div>

			<div className="px-2 pb-2">
				<UsageProgress
					ariaLabel={section.progressLabel}
					percent={section.percent}
					severity={section.severity}
				/>
			</div>

			<div
				className={cn(
					"px-2 text-xs leading-5 text-content-secondary",
					section.secondaryDetail ? "pb-1.5" : "pb-2",
				)}
			>
				<div className="flex items-start gap-1.5">
					<span className="min-w-0 flex-1">{section.detail}</span>
					{section.tooltip && (
						<TooltipProvider delayDuration={300}>
							<Tooltip>
								<TooltipTrigger asChild>
									<button
										type="button"
										className="mt-0.5 inline-flex size-3.5 shrink-0 cursor-help items-center justify-center rounded-sm border-none bg-transparent p-0 text-content-secondary/70 outline-none transition-colors hover:text-content-primary focus-visible:ring-2 focus-visible:ring-content-link"
										aria-label={`${section.title} help`}
									>
										<InfoIcon className="size-3.5" />
									</button>
								</TooltipTrigger>
								<TooltipContent
									side="right"
									sideOffset={4}
									className="max-w-48 text-xs"
								>
									{section.tooltip}
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
				</div>
			</div>

			{section.secondaryDetail && (
				<div className="px-2 pb-2 text-xs text-content-secondary">
					{section.secondaryDetail}
				</div>
			)}
		</>
	);
};

const UsageProgress: FC<{
	ariaLabel: string;
	percent: number;
	severity?: UsageSeverity;
	size?: "default" | "compact";
	className?: string;
}> = ({
	ariaLabel,
	percent,
	severity = "normal",
	size = "default",
	className,
}) => {
	const clampedPercent = clampPercent(percent);

	return (
		<div
			role="progressbar"
			aria-label={ariaLabel}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round(clampedPercent)}
			className={cn(
				size === "compact" ? "h-1" : "h-1.5",
				"overflow-hidden rounded-full bg-surface-tertiary",
				className,
			)}
		>
			<div
				className={cn(
					"h-full rounded-full transition-all duration-300 ease-out",
					getProgressClassName(severity),
				)}
				style={{ width: `${clampedPercent}%` }}
			/>
		</div>
	);
};

function getPercent(used: number, budget: number): number {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return 0;
	}
	if (budget === 0) {
		return used > 0 ? 100 : 0;
	}
	return clampPercent((used / budget) * 100);
}

function clampPercent(percent: number): number {
	if (!Number.isFinite(percent)) {
		return 0;
	}
	return Math.min(Math.max(percent, 0), 100);
}

function getSeverity(used: number, budget: number): UsageSeverity {
	if (!Number.isFinite(used) || !Number.isFinite(budget) || budget < 0) {
		return "normal";
	}
	if (budget === 0) {
		return used > 0 ? "exceeded" : "normal";
	}
	if (used >= budget) {
		return "exceeded";
	}
	return used / budget >= 0.85 ? "warning" : "normal";
}

function getProgressClassName(severity: UsageSeverity): string {
	switch (severity) {
		case "exceeded":
			return "bg-content-destructive";
		case "warning":
			return "bg-content-warning";
		case "normal":
			return "bg-content-secondary";
	}
}

function getTextClassName(severity: UsageSeverity = "normal"): string {
	switch (severity) {
		case "exceeded":
			return "text-content-destructive";
		case "warning":
			return "text-content-warning";
		case "normal":
			return "text-content-secondary";
	}
}

function getWorkspaceCount(count: number | undefined): number | undefined {
	if (count === undefined || !Number.isFinite(count) || count < 0) {
		return undefined;
	}
	return count;
}

function formatNumber(value: number): string {
	return numberFormatter.format(value);
}
