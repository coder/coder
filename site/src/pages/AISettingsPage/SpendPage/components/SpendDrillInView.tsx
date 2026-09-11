import { ChevronLeftIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { queryWithTimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { RetentionNotice } from "./RetentionNotice";
import type { SpendDimensions } from "./SpendFilters";
import { SpendSectionHeader } from "./SpendSectionHeader";
import { SpendSummaryView } from "./SpendSummaryView";

interface SpendDrillInViewProps {
	selectedUser: TypesGen.User | null;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	onBack: () => void;
	filters: ReactNode;
	dimensions: SpendDimensions;
	queryDateRange: DateRangeValue;
	summaryData: TypesGen.AIGatewaySpendUserSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

type AppliedWindow = Pick<
	TypesGen.AIGatewaySpendUserSummary,
	"start_date" | "end_date"
>;

// Links to the AI Sessions page for the same user, window, and filters. The
// user ID rather than the username keeps the link pointing at this account
// even if the username is later reused. The sessions page applies no retention
// clamp, so the link waits for the applied window from the summary rather than
// using the requested one.
const sessionsHref = (
	userId: string,
	dimensions: SpendDimensions,
	applied: AppliedWindow,
) => {
	const filter = queryWithTimeRange(
		{ ...dimensions, initiator: userId },
		{ start: new Date(applied.start_date), end: new Date(applied.end_date) },
	);
	return `/ai-gateway/sessions?${useFilterParamsKey}=${encodeURIComponent(filter)}`;
};

// A window that lies entirely outside retained data comes back collapsed to
// start_date == end_date, which the sessions filter rejects, so there is
// nothing to link to.
const hasEmptyAppliedWindow = (applied: AppliedWindow) =>
	new Date(applied.start_date).getTime() >=
	new Date(applied.end_date).getTime();

export const SpendDrillInView: FC<SpendDrillInViewProps> = ({
	selectedUser,
	isLoading,
	error,
	onRetry,
	onBack,
	filters,
	dimensions,
	queryDateRange,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	const header = (
		<>
			<div>
				<button
					type="button"
					onClick={onBack}
					className="mb-4 inline-flex cursor-pointer items-center gap-0.5 border-0 bg-transparent p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
				>
					<ChevronLeftIcon className="size-4" />
					Back
				</button>
				<SpendSectionHeader
					title="Spend details"
					description="AI Gateway spend for a single user in the selected period and filters."
				/>
			</div>
			{filters}
		</>
	);

	if (isLoading) {
		return (
			<div className="space-y-6">
				{header}
				<div
					role="status"
					aria-label="Loading user details"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			</div>
		);
	}

	if (!selectedUser) {
		return (
			<div className="space-y-6">
				{header}
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{getErrorMessage(error, "Failed to load user profile.")}
					</p>
					<Button variant="outline" size="sm" type="button" onClick={onRetry}>
						Retry
					</Button>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{header}
			<div className="flex items-center justify-between gap-4 rounded-lg bg-surface-secondary px-4 py-3">
				<AvatarData
					title={selectedUser.name || selectedUser.username}
					subtitle={`@${selectedUser.username}`}
					src={selectedUser.avatar_url}
					imgFallbackText={selectedUser.username}
				/>
				{summaryData && !hasEmptyAppliedWindow(summaryData) && (
					<Link asChild showExternalIcon={false} size="sm">
						<RouterLink
							to={sessionsHref(selectedUser.id, dimensions, summaryData)}
						>
							View sessions
						</RouterLink>
					</Link>
				)}
			</div>
			{summaryData && (
				<RetentionNotice
					requestedStart={queryDateRange.startDate}
					applied={summaryData}
				/>
			)}
			<SpendSummaryView
				key={selectedUser.id}
				summary={summaryData}
				isLoading={isSummaryLoading}
				error={summaryError}
				onRetry={onSummaryRetry}
			/>
		</div>
	);
};
