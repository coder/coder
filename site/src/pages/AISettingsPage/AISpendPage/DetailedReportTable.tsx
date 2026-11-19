import { cn } from "cn";
import type { UseQueryResult } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import LinearProgress from "#/components/LinearProgress/LinearProgress";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { SpendAmount } from "#/pages/AISettingsPage/SpendPage/components/SpendAmount";
import { formatCostMicros } from "#/utils/currency";
import { UnpricedModelsTooltip } from "./UnpricedModelsTooltip";

const BuildingReport: React.FC<{ className?: string }> = ({ className }) => (
	<div
		role="status"
		className={cn(
			"flex flex-col items-center justify-center gap-3 px-6 text-sm text-content-secondary",
			className,
		)}
	>
		<LinearProgress
			data-pixel="ignore"
			variant="indeterminate"
			value={0}
			className="max-w-xl"
		/>
		Building your report… stay on this page
	</div>
);

export type DetailedReportQuery =
	PaginationResult<TypesGen.OrganizationAISpendReport> &
		Pick<
			UseQueryResult<TypesGen.OrganizationAISpendReport>,
			"isFetching" | "error" | "refetch"
		>;

type DetailedReportTableProps = {
	reportQuery: DetailedReportQuery;
};

export const DetailedReportTable: React.FC<DetailedReportTableProps> = ({
	reportQuery,
}) => {
	const retryButton = (
		<Button
			variant="outline"
			size="sm"
			onClick={() => void reportQuery.refetch()}
		>
			Retry
		</Button>
	);

	const report = reportQuery.data;
	if (report === undefined) {
		if (reportQuery.error !== null) {
			return <ErrorAlert error={reportQuery.error} actions={retryButton} />;
		}
		// The same height as the empty state before the report runs.
		return (
			<BuildingReport className="min-h-96 rounded-lg border border-border" />
		);
	}

	return (
		<div className="flex flex-col gap-6">
			{reportQuery.error !== null && (
				<ErrorAlert error={reportQuery.error} actions={retryButton} />
			)}
			<div className="relative flex flex-col gap-6">
				{reportQuery.isFetching && (
					<BuildingReport className="absolute inset-0 z-10 bg-surface-primary/80" />
				)}
				<div className="flex flex-col gap-1">
					<span className="text-sm text-content-secondary">Total spend</span>
					<span className="text-2xl font-semibold text-content-primary">
						<SpendAmount
							scope="organization"
							costMicros={report.totals.cost_micros}
							hasUnpricedUsage={report.totals.has_unpriced_usage}
						/>
					</span>
				</div>
				<PaginationContainer
					query={reportQuery}
					paginationUnitLabel={{ singular: "user", plural: "users" }}
				>
					<Table aria-label="Spend by user">
						<TableHeader>
							<TableRow>
								<TableHead>User</TableHead>
								<TableHead className="text-right">Spend</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{report.users.length === 0 ? (
								<TableEmpty message="No AI Gateway spend found" isCompact />
							) : (
								report.users.map((user) => {
									const displayName = user.name || user.username;
									const unpricedModels = user.model_spend.filter(
										(model) => model.has_unpriced_usage,
									);
									return (
										<TableRow key={user.user_id}>
											<TableHead scope="row">
												<AvatarData
													truncate
													title={displayName}
													subtitle={`@${user.username}`}
													src={user.avatar_url}
													imgFallbackText={user.username}
												/>
											</TableHead>
											<TableCell className="text-right tabular-nums">
												{formatCostMicros(user.cost_micros)}
											</TableCell>
											<TableCell>
												{unpricedModels.length > 0 && (
													<UnpricedModelsTooltip
														displayName={displayName}
														models={unpricedModels}
													/>
												)}
											</TableCell>
										</TableRow>
									);
								})
							)}
						</TableBody>
					</Table>
				</PaginationContainer>
			</div>
		</div>
	);
};
