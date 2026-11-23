import { DownloadIcon, PlayIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { DetailedReportFilter } from "./DetailedReportFilter";
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
};

export const DetailedReportTab: React.FC<DetailedReportTabProps> = ({
	organizationId,
	now,
	days,
	onDaysChange,
	filter,
	onFilterChange,
	canFilter,
}) => {
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
				{/* TODO(AIGOV-731): run the report for the picked days. */}
				<Button disabled>
					<PlayIcon aria-hidden />
					Run report
				</Button>
				{/* TODO(AIGOV-731): export the report once it has run. */}
				<Button variant="outline" disabled className="sm:ml-auto">
					<DownloadIcon aria-hidden />
					Export CSV
				</Button>
			</div>
			<EmptyState
				className="rounded-lg border border-border"
				message="Create a detailed report"
				description="See spend for every user, broken down by provider, model, and client. Large reports may take a few minutes."
			/>
		</div>
	);
};
