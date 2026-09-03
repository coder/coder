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
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { BackButton } from "./BackButton";
import { SpendSectionHeader } from "./SpendSectionHeader";
import { SpendSummaryView } from "./SpendSummaryView";

interface SpendDrillInViewProps {
	selectedUser: TypesGen.User | null;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	onBack: () => void;
	displayDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	dateRangeLabel: string;
	summaryData: TypesGen.AIGatewaySpendUserSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

// Mirrors the AI Sessions page filter syntax so the link lands on that
// user's sessions.
const sessionsHref = (username: string) =>
	`/ai-gateway/sessions?filter=${encodeURIComponent(`initiator:${username}`)}`;

export const SpendDrillInView: FC<SpendDrillInViewProps> = ({
	selectedUser,
	isLoading,
	error,
	onRetry,
	onBack,
	displayDateRange,
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
					<Link asChild showExternalIcon={false} size="sm">
						<RouterLink to={sessionsHref(selectedUser.username)}>
							View sessions
						</RouterLink>
					</Link>
				</div>
			</div>
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
