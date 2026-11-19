import { EllipsisVerticalIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import { Link } from "react-router";
import type {
	AISpendPeriodWindow,
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
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { queryWithTimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { formatCostMicros } from "#/utils/currency";
import { CostDetailsDialog } from "./CostDetailsDialog";
import { OverviewCard } from "./OverviewCard";
import { summarizeTopSpenders, TOP_SPENDERS_COUNT } from "./topSpenders";
import { UnpricedModelsTooltip } from "./UnpricedModelsTooltip";

type TopSpendersCardProps = {
	/** Undefined while the spend loads. */
	overview: OrganizationAISpendOverview | undefined;
	/** The listed users' spend details, undefined while they load. */
	topSpenderDetails: readonly OrganizationAISpendUser[] | undefined;
	topSpenderDetailsError: Error | null;
	organizationName: string;
	/** The sessions page needs read access to every user's sessions. */
	canViewSessions: boolean;
};

export const TopSpendersCard: React.FC<TopSpendersCardProps> = ({
	overview,
	topSpenderDetails,
	topSpenderDetailsError,
	organizationName,
	canViewSessions,
}) => {
	const titleId = useId();
	const summary = overview && summarizeTopSpenders(overview);
	// Undefined while the details load, null when the response has no entry
	// for the user.
	const findUserDetails = (userId: string) => {
		if (topSpenderDetails === undefined) {
			return undefined;
		}
		return (
			topSpenderDetails.find((spender) => spender.user_id === userId) ?? null
		);
	};

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
							details={findUserDetails(user.user_id)}
							detailsError={topSpenderDetailsError}
							period={overview}
							organizationName={organizationName}
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
	/** Undefined while the details load, null when the user has none. */
	details: OrganizationAISpendUser | null | undefined;
	detailsError: Error | null;
	period: AISpendPeriodWindow;
	organizationName: string;
	canViewSessions: boolean;
};

const TopSpenderRow: React.FC<TopSpenderRowProps> = ({
	user,
	details,
	detailsError,
	period,
	organizationName,
	canViewSessions,
}) => {
	const [isCostDetailsOpen, setIsCostDetailsOpen] = useState(false);
	const menuTriggerRef = useRef<HTMLButtonElement>(null);
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
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						ref={menuTriggerRef}
						size="icon"
						variant="subtle"
						aria-label={`Open menu for ${displayName}`}
					>
						<EllipsisVerticalIcon aria-hidden />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onClick={() => setIsCostDetailsOpen(true)}>
						Cost details
					</DropdownMenuItem>
					{canViewSessions && (
						<DropdownMenuItem asChild>
							<Link
								to={`/ai-gateway/sessions?filter=${encodeURIComponent(sessionsFilter)}`}
							>
								User AI session logs
							</Link>
						</DropdownMenuItem>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
			<CostDetailsDialog
				open={isCostDetailsOpen}
				onOpenChange={setIsCostDetailsOpen}
				onCloseAutoFocus={(event) => {
					// The dialog opens from a menu item, which is gone once it closes.
					event.preventDefault();
					menuTriggerRef.current?.focus();
				}}
				user={user}
				details={details}
				detailsError={detailsError}
				period={period}
				organizationName={organizationName}
			/>
		</li>
	);
};
