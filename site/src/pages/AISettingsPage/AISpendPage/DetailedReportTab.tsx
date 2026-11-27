import { DownloadIcon, PlayIcon, XIcon } from "lucide-react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Spinner } from "#/components/Spinner/Spinner";
import { DetailedReportFilter } from "./DetailedReportFilter";
import {
	type DetailedReportQuery,
	DetailedReportTable,
} from "./DetailedReportTable";
import { SpendDaysPicker } from "./SpendDaysPicker";
import type { SpendDays } from "./spendDays";

type DetailedReportTabProps = {
	organizationId: string;
	organizationName: string;
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	minDay: string | undefined;
	isRetentionLoading: boolean;
	filter: string;
	onFilterChange: (filter: string) => void;
	/** The filter options and session logs need read access to every session. */
	canViewSessions: boolean;
	reportQuery: DetailedReportQuery;
	hasRunReport: boolean;
	/** The days or filter changed since the report last ran. */
	isReportOutdated: boolean;
	onRunReport: () => void;
	onCancelReport: () => void;
	/** The last run returned rows to export. */
	canExportReport: boolean;
	isExportingReport: boolean;
	onExportReport: () => void;
};

export const DetailedReportTab: React.FC<DetailedReportTabProps> = ({
	organizationId,
	organizationName,
	now,
	days,
	onDaysChange,
	minDay,
	isRetentionLoading,
	filter,
	onFilterChange,
	canViewSessions,
	reportQuery,
	hasRunReport,
	isReportOutdated,
	onRunReport,
	onCancelReport,
	canExportReport,
	isExportingReport,
	onExportReport,
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
				{canViewSessions && (
					<DetailedReportFilter
						organizationId={organizationId}
						value={filter}
						onChange={onFilterChange}
					/>
				)}
				<SpendDaysPicker
					now={now}
					days={days}
					onDaysChange={onDaysChange}
					minDay={minDay}
					disabled={isRetentionLoading}
				/>
				{isReportRunning ? (
					<Button variant="outline" onClick={onCancelReport}>
						<XIcon aria-hidden />
						Cancel
					</Button>
				) : (
					<Button
						variant={runReportVariant}
						onClick={onRunReport}
						disabled={isRetentionLoading}
					>
						<PlayIcon aria-hidden />
						Run report
					</Button>
				)}
				<Button
					variant="outline"
					disabled={!canExportReport || isExportingReport}
					onClick={onExportReport}
					className="sm:ml-auto"
				>
					<Spinner loading={isExportingReport}>
						<DownloadIcon aria-hidden />
					</Spinner>
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
					<DetailedReportTable
						reportQuery={reportQuery}
						organizationName={organizationName}
						canViewSessions={canViewSessions}
					/>
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
