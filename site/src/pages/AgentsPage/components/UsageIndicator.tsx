import dayjs from "dayjs";
import { CoinsIcon, InfoIcon, ServerIcon } from "lucide-react";
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
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getDefaultOrganizationName,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { getUsageLimitPeriodLabel } from "#/pages/AISettingsPage/SpendPage/components/ChatCostSummaryView";
import {
	clampPercentage,
	getSeverity,
	severityRingClassName,
	severityTextClassName,
	type UsageSeverity,
	usageProgressPercentage,
} from "#/utils/budget";
import { cn } from "#/utils/cn";
import { formatCostMicros } from "#/utils/currency";
import { SvgRingProgress } from "./SvgRingProgress";

type UsageSectionData = {
	id: string;
	title: string;
	progressLabel: string;
	percent: number;
	detail: ReactNode;
	icon: ReactNode;
	hoverLabel: string;
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
	const organizationName = getDefaultOrganizationName(organizations);
	const username = user.username;
	const { data: quota, isError: isQuotaError } = useQuery({
		...workspaceQuota(organizationName, username),
		refetchInterval: 60_000,
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
			title: `${periodLabel} usage`,
			progressLabel: `${periodLabel} spend usage`,
			percent: usageProgressPercentage(currentSpend, spendLimit),
			severity: getSeverity(currentSpend, spendLimit),
			icon: <CoinsIcon className="size-3.5" />,
			hoverLabel: `Spend ${formatCostMicros(currentSpend)}`,
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

		const workspaceHoverLabel =
			quota.budget > 0
				? `Workspaces ${formatNumber(creditsConsumed)}/${formatNumber(quota.budget)}`
				: `Workspaces ${formatNumber(creditsConsumed)}`;

		sections.push({
			id: "workspace-quota",
			title: "Workspace quota",
			progressLabel: "Workspace quota usage",
			percent: usageProgressPercentage(creditsConsumed, quota.budget),
			severity: getSeverity(creditsConsumed, quota.budget),
			icon: <ServerIcon className="size-3.5" />,
			hoverLabel: workspaceHoverLabel,
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
	const triggerAriaLabel =
		sections.length > 1 ? "Usage" : (sections[0]?.title ?? "Usage");

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					aria-label={triggerAriaLabel}
					className="flex shrink-0 self-stretch items-center justify-center border-none bg-transparent px-3 cursor-pointer select-none transition-colors hover:bg-surface-tertiary/50 outline-none"
				>
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

const RING_SIZE = 28;
const RING_STROKE = 1;

const UsageTriggerProgress: FC<{ sections: readonly UsageSectionData[] }> = ({
	sections,
}) => {
	return (
		<TooltipProvider delayDuration={150}>
			<div className="flex shrink-0 items-center gap-2">
				{sections.map((section) => (
					<Tooltip key={section.id}>
						<TooltipTrigger asChild>
							<div>
								<UsageRingProgress
									ariaLabel={section.progressLabel}
									percent={section.percent}
									severity={section.severity}
									icon={section.icon}
								/>
							</div>
						</TooltipTrigger>
						<TooltipContent side="top" sideOffset={6}>
							{section.hoverLabel}
						</TooltipContent>
					</Tooltip>
				))}
			</div>
		</TooltipProvider>
	);
};

const UsageRingProgress: FC<{
	ariaLabel: string;
	percent: number;
	severity?: UsageSeverity;
	icon: ReactNode;
}> = ({ ariaLabel, percent, severity = "normal", icon }) => {
	const clampedPercent = clampPercentage(percent);

	return (
		<div
			role="progressbar"
			aria-label={ariaLabel}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round(clampedPercent)}
			className="relative flex shrink-0 items-center justify-center"
			style={{ width: RING_SIZE, height: RING_SIZE }}
		>
			<SvgRingProgress
				size={RING_SIZE}
				strokeWidth={RING_STROKE}
				percent={clampedPercent}
				progressClassName={severityRingClassName(severity)}
			/>
			<span
				aria-hidden="true"
				className={cn(
					"absolute inset-0 flex items-center justify-center",
					severityTextClassName(severity),
				)}
			>
				{icon}
			</span>
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
					className={cn(
						"shrink-0 text-xs",
						severityTextClassName(section.severity),
					)}
				>
					{roundedPercent}%
				</span>
			</div>

			<div className="px-2 pb-2">
				<UsageBar
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

function getWorkspaceCount(count: number | undefined): number | undefined {
	if (count === undefined || !Number.isFinite(count) || count < 0) {
		return undefined;
	}
	return count;
}

function formatNumber(value: number): string {
	return numberFormatter.format(value);
}
