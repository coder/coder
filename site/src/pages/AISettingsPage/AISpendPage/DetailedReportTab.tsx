import { DownloadIcon, PlayIcon, XIcon } from "lucide-react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { DetailedReportFilter } from "./DetailedReportFilter";
import {
	type DetailedReportQuery,
	DetailedReportTable,
} from "./DetailedReportTable";
import { SpendDaysPicker } from "./SpendDaysPicker";
import type { SpendDays } from "./spendDays";

type DetailedReportTabProps = {
	organizationId: string;
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	filter: string;
	onFilterChange: (filter: string) => void;
	/** The filter options come from AI Gateway endpoints that need read access to every session. */
	canFilter: boolean;
	reportQuery: DetailedReportQuery;
	hasRunReport: boolean;
	/** The days or filter changed since the report last ran. */
	isReportOutdated: boolean;
	onRunReport: () => void;
	onCancelReport: () => void;
};

export const DetailedReportTab: React.FC<DetailedReportTabProps> = ({
	organizationId,
	now,
	days,
	onDaysChange,
	filter,
	onFilterChange,
	canFilter,
	reportQuery,
	hasRunReport,
	isReportOutdated,
	onRunReport,
	onCancelReport,
}) => {
	const isReportRunning = hasRunReport && reportQuery.isFetching;
	const runReportVariant =
		!hasRunReport || isReportOutdated ? "default" : "outline";

	return (
		<div className="flex flex-col gap-6">
			<section className="flex flex-col gap-2">
				<h2 className="m-0 text-xl font-semibold">
					Generate a detailed report
				</h2>
				<p className="m-0 max-w-3xl text-sm text-content-secondary">
					Spend for every user, with the providers, models, and clients they
					used. Large reports can take a while to generate.
				</p>
			</section>
			<div className="flex flex-wrap items-center gap-3">
				{canFilter && (
					<DetailedReportFilter
						organizationId={organizationId}
						value={filter}
						onChange={onFilterChange}
					/>
				)}
				<SpendDaysPicker now={now} days={days} onDaysChange={onDaysChange} />
				{isReportRunning ? (
					<Button variant="outline" onClick={onCancelReport}>
						<XIcon aria-hidden />
						Cancel
					</Button>
				) : (
					<Button variant={runReportVariant} onClick={onRunReport}>
						<PlayIcon aria-hidden />
						Run report
					</Button>
				)}
				{/* TODO(AIGOV-731): export the report once it has run. */}
				<Button variant="outline" disabled className="sm:ml-auto">
					<DownloadIcon aria-hidden />
					Export CSV
				</Button>
			</div>
			{hasRunReport ? (
				<>
					{isReportOutdated && (
						<Alert severity="info">
							The filters or dates changed since the last run. Run the report to
							update the results.
						</Alert>
					)}
					<DetailedReportTable reportQuery={reportQuery} />
				</>
			) : (
				<EmptyState
					className="rounded-lg border border-border"
					message="Create a detailed report"
					description="See spend for every user, broken down by provider, model, and client. Large reports may take a few minutes."
				/>
			)}
		</div>
	);
};
