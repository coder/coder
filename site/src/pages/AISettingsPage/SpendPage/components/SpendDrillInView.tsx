import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { queryWithTimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { BackButton } from "./BackButton";
import { RetentionNotice } from "./RetentionNotice";
import { SpendSectionHeader } from "./SpendSectionHeader";
import { SpendSummaryView } from "./SpendSummaryView";

interface SpendDrillInViewProps {
	selectedUser: TypesGen.User | null;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	onBack: () => void;
	displayDateRange: DateRangeValue;
	queryDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	dateRangeLabel: string;
	summaryData: TypesGen.AIGatewaySpendUserSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

type AppliedWindow = Pick<
	TypesGen.AIGatewaySpendUserSummary,
	"start_date" | "end_date"
>;

// Links to the AI Sessions page for the same user and window. The user ID
// rather than the username keeps the link pointing at this account even if
// the username is later reused. The sessions page applies no retention clamp,
// so the link waits for the applied window from the summary rather than using
// the requested one.
const sessionsHref = (userId: string, applied: AppliedWindow) => {
	const filter = queryWithTimeRange(
		{ initiator: userId },
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
	displayDateRange,
	queryDateRange,
	onDateRangeChange,
	dateRangeLabel,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	const header = (
		<div>
			<BackButton onClick={onBack} />
			<SpendSectionHeader
				title="Spend details"
				description="AI Gateway spend for a single user in the selected date range."
				actions={
					<DateRangePicker
						value={displayDateRange}
						onChange={onDateRangeChange}
					/>
				}
			/>
		</div>
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
				<div className="flex min-w-0 flex-col items-end gap-1 text-xs text-content-secondary">
					<div>{dateRangeLabel}</div>
					{summaryData && !hasEmptyAppliedWindow(summaryData) && (
						<Link asChild showExternalIcon={false} size="sm">
							<RouterLink to={sessionsHref(selectedUser.id, summaryData)}>
								View sessions
							</RouterLink>
						</Link>
					)}
				</div>
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
