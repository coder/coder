import { EllipsisVerticalIcon, TriangleAlertIcon } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router";
import type {
	AISpendPeriodWindow,
	OrganizationAISpendModel,
	OrganizationAISpendOverview,
	OrganizationAISpendOverviewUser,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link as DocsLink } from "#/components/Link/Link";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipMessage,
	TooltipTitle,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { queryWithTimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { formatCostMicros } from "#/utils/currency";
import { docs } from "#/utils/docs";
import { OverviewCard } from "./OverviewCard";
import { summarizeTopSpenders, TOP_SPENDERS_COUNT } from "./topSpenders";

type TopSpendersCardProps = {
	/** Undefined while the spend loads. */
	overview: OrganizationAISpendOverview | undefined;
	/** The listed users' spend details, undefined while they load. */
	topSpenderDetails: readonly OrganizationAISpendUser[] | undefined;
	/** The sessions page needs read access to every user's sessions. */
	canViewSessions: boolean;
};

export const TopSpendersCard: React.FC<TopSpendersCardProps> = ({
	overview,
	topSpenderDetails,
	canViewSessions,
}) => {
	const titleId = useId();
	const summary = overview && summarizeTopSpenders(overview);

	return (
		<OverviewCard aria-labelledby={titleId} className="gap-6">
			<div className="flex flex-col items-start gap-3">
				<div className="flex items-center gap-2">
					<h2 id={titleId} className="m-0 text-xl font-semibold">
						Top spenders
					</h2>
					<InfoTooltip ariaLabel="About top spenders">
						The users with the most AI spend in the selected period.
					</InfoTooltip>
				</div>
				{summary && (
					<p className="m-0 text-sm text-content-secondary">{summary}</p>
				)}
				{/* TODO(AIGOV-731): open the Detailed report tab once it is built. */}
				<Button variant="outline" size="sm" disabled>
					Detailed report
				</Button>
			</div>
			{overview === undefined ? (
				<div className="flex flex-col gap-2">
					{Array.from({ length: TOP_SPENDERS_COUNT }, (_, i) => (
						<Skeleton key={i} className="h-12" />
					))}
				</div>
			) : overview.users.length === 0 ? (
				<div className="flex min-h-44 flex-1 items-center justify-center text-sm font-medium text-content-secondary">
					No users spent on AI in this period.
				</div>
			) : (
				<ul
					aria-labelledby={titleId}
					className="m-0 flex list-none flex-col rounded-lg border border-border p-0"
				>
					{overview.users.map((user) => (
						<TopSpenderRow
							key={user.user_id}
							user={user}
							details={topSpenderDetails?.find(
								(spender) => spender.user_id === user.user_id,
							)}
							period={overview}
							canViewSessions={canViewSessions}
						/>
					))}
				</ul>
			)}
		</OverviewCard>
	);
};

type TopSpenderRowProps = {
	user: OrganizationAISpendOverviewUser;
	details: OrganizationAISpendUser | undefined;
	period: AISpendPeriodWindow;
	canViewSessions: boolean;
};

const TopSpenderRow: React.FC<TopSpenderRowProps> = ({
	user,
	details,
	period,
	canViewSessions,
}) => {
	const displayName = user.name || user.username;
	const unpricedModels =
		details?.model_spend.filter((model) => model.has_unpriced_usage) ?? [];
	const sessionsFilter = queryWithTimeRange(
		{ initiator: user.username },
		{
			start: new Date(period.period_start),
			end: new Date(period.period_end),
		},
	);

	return (
		<li className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
			<Avatar src={user.avatar_url} fallback={displayName} />
			<span className="min-w-0 flex-1 truncate text-sm">{displayName}</span>
			<span className="text-sm tabular-nums text-content-secondary">
				{formatCostMicros(user.spend_micros)}
			</span>
			{/* A fixed slot keeps the amounts aligned in rows without a warning. */}
			<div className="flex w-10 justify-end">
				{unpricedModels.length > 0 && (
					<UnpricedModelsTooltip
						displayName={displayName}
						models={unpricedModels}
					/>
				)}
			</div>
			{/* TODO(AIGOV-734): add Cost details once the per-user report endpoint lands. */}
			{canViewSessions && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							size="icon"
							variant="subtle"
							aria-label={`Open menu for ${displayName}`}
						>
							<EllipsisVerticalIcon aria-hidden />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem asChild>
							<Link
								to={`/ai-gateway/sessions?filter=${encodeURIComponent(sessionsFilter)}`}
							>
								User AI session logs
							</Link>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</li>
	);
};

type UnpricedModelsTooltipProps = {
	displayName: string;
	models: readonly OrganizationAISpendModel[];
};

const UnpricedModelsTooltip: React.FC<UnpricedModelsTooltipProps> = ({
	displayName,
	models,
}) => {
	const isSingle = models.length === 1;
	return (
		<Tooltip interactive>
			<TooltipTrigger
				aria-label={`${models.length} ${isSingle ? "model" : "models"} without pricing`}
				className="gap-1 text-sm tabular-nums text-content-warning"
			>
				<TriangleAlertIcon aria-hidden className="size-icon-sm" />
				{models.length}
			</TooltipTrigger>
			<TooltipContent className="max-w-[min(20rem,var(--radix-popper-available-width))]">
				<TooltipTitle>Model pricing missing</TooltipTitle>
				<TooltipMessage>
					{isSingle
						? `${models[0].name} used by ${displayName} has no pricing configured. Its usage isn't included, so total spend may be higher.`
						: `${models.length} models used by ${displayName} have no pricing configured. Their usage isn't included, so total spend may be higher.`}
				</TooltipMessage>
				<DocsLink
					size="sm"
					href={docs(
						"/ai-coder/ai-gateway/cost-controls#configure-model-prices",
					)}
					target="_blank"
					rel="noreferrer"
				>
					View docs
				</DocsLink>
			</TooltipContent>
		</Tooltip>
	);
};
