import type { FC } from "react";
import type { UseQueryResult } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { ClientsBadge } from "#/pages/AIBridgePage/ClientsBadge";
import { ModelsBadge } from "#/pages/AIBridgePage/ModelsBadge";
import { ProvidersBadge } from "#/pages/AIBridgePage/ProvidersBadge";
import { SpendAmount } from "./SpendAmount";

export type SpendReportQuery =
	PaginationResult<TypesGen.OrganizationAISpendReport> &
		Pick<
			UseQueryResult<TypesGen.OrganizationAISpendReport, unknown>,
			"isLoading" | "isFetching" | "error" | "refetch"
		>;

type SpendUsersTableProps = {
	reportQuery: SpendReportQuery;
};

export const SpendUsersTable: FC<SpendUsersTableProps> = ({ reportQuery }) => {
	const retryButton = (
		<Button
			variant="outline"
			size="sm"
			type="button"
			onClick={() => void reportQuery.refetch()}
		>
			Retry
		</Button>
	);

	const report = reportQuery.data;
	if (report === undefined) {
		return reportQuery.error != null ? (
			<div className="flex min-h-60 flex-col items-center justify-center gap-4 text-center">
				<ErrorAlert error={reportQuery.error} />
				{retryButton}
			</div>
		) : (
			<div
				role="status"
				aria-label="Loading spend"
				className="flex min-h-60 items-center justify-center"
			>
				<Spinner size="lg" loading className="text-content-secondary" />
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{reportQuery.error != null && (
				<ErrorAlert error={reportQuery.error} actions={retryButton} />
			)}
			<div className="relative flex flex-col gap-6">
				{reportQuery.isFetching && (
					<div
						role="status"
						aria-label="Refreshing spend"
						className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
					>
						<Spinner size="lg" loading className="text-content-secondary" />
					</div>
				)}
				<SpendTotal report={report} />
				<PaginationContainer query={reportQuery} paginationUnitLabel="users">
					<Table
						aria-label="Spend by user"
						className="table-fixed"
						// Beside the settings sidebar the table is far narrower than the
						// viewport, so the column widths follow the wrapper, not the screen.
						wrapperClassName="@container"
					>
						<TableHeader>
							<TableRow>
								<TableHead className="w-36 @3xl:w-48">User</TableHead>
								<TableHead className="w-28 @3xl:w-36">Providers</TableHead>
								<TableHead className="w-28 @3xl:w-36">Models</TableHead>
								<TableHead className="w-28 @3xl:w-36">Clients</TableHead>
								<TableHead className="w-24 text-right @3xl:w-28">
									Spend
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody size="lg">
							{report.users.length === 0 ? (
								<TableEmpty message="No AI Gateway spend found" isCompact />
							) : (
								report.users.map((user) => (
									<SpendUserRow key={user.user_id} user={user} />
								))
							)}
						</TableBody>
					</Table>
				</PaginationContainer>
			</div>
		</div>
	);
};

type SpendUserRowProps = {
	user: TypesGen.OrganizationAISpendUser;
};

const SpendUserRow: FC<SpendUserRowProps> = ({ user }) => (
	<TableRow>
		{/* The row header gives the count badges and warning their user. */}
		<TableHead
			scope="row"
			className="border-0 border-t border-border border-solid"
		>
			<AvatarData
				truncate
				title={user.name || user.username}
				subtitle={`@${user.username}`}
				src={user.avatar_url}
				imgFallbackText={user.username}
			/>
		</TableHead>
		<TableCell>
			<ProvidersBadge providers={user.providers} />
		</TableCell>
		<TableCell>
			<ModelsBadge models={user.models} />
		</TableCell>
		<TableCell>
			<ClientsBadge clients={user.clients} />
		</TableCell>
		<TableCell className="text-right">
			<SpendAmount
				scope={{ user: user.name || user.username }}
				costMicros={user.cost_micros}
				unpricedUsageCount={user.unpriced_usage_count}
			/>
		</TableCell>
	</TableRow>
);

type SpendTotalProps = {
	report: TypesGen.OrganizationAISpendReport;
};

// The total covers every matching user in the period, not only the page.
const SpendTotal: FC<SpendTotalProps> = ({ report }) => (
	<div className="flex flex-col gap-1">
		<span className="text-sm text-content-secondary">Total spend</span>
		<span className="text-2xl font-semibold text-content-primary">
			<SpendAmount
				scope="organization"
				costMicros={report.totals.cost_micros}
				unpricedUsageCount={report.totals.unpriced_usage_count}
			/>
		</span>
	</div>
);
