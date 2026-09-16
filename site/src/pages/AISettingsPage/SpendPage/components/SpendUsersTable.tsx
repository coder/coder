import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { ClientsBadge } from "#/pages/AIBridgePage/ClientsBadge";
import { ProvidersBadge } from "#/pages/AIBridgePage/ProvidersBadge";
import { formatCostMicros } from "#/utils/currency";
import type { SpendUsersQuery } from "../SpendPageView";
import { CostCell } from "./CostCell";

interface SpendUsersTableProps {
	usersQuery: SpendUsersQuery;
}

export const SpendUsersTable: FC<SpendUsersTableProps> = ({ usersQuery }) => {
	const retryButton = (
		<Button
			variant="outline"
			size="sm"
			type="button"
			onClick={() => void usersQuery.refetch()}
		>
			Retry
		</Button>
	);

	return (
		<div className="space-y-6">
			{usersQuery.isLoading && (
				<div
					role="status"
					aria-label="Loading spend"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}
			{usersQuery.error != null && !usersQuery.data && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<ErrorAlert error={usersQuery.error} />
					{retryButton}
				</div>
			)}
			{usersQuery.data && (
				<>
					{usersQuery.error != null && (
						<ErrorAlert error={usersQuery.error} actions={retryButton} />
					)}
					<div className="relative">
						{usersQuery.isFetching && !usersQuery.isLoading && (
							<div
								role="status"
								aria-label="Refreshing spend"
								className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
							>
								<Spinner size="lg" loading className="text-content-secondary" />
							</div>
						)}
						{usersQuery.data.users.length === 0 ? (
							<p className="py-12 text-center text-content-secondary">
								No AI Gateway spend matches these filters.
							</p>
						) : (
							<div className="flex flex-col gap-6">
								<SpendTotal report={usersQuery.data} />
								<PaginationContainer
									query={usersQuery}
									paginationUnitLabel="users"
								>
									<Table aria-label="Spend by user">
										<TableHeader>
											<TableRow>
												<TableHead>User</TableHead>
												<TableHead>Providers</TableHead>
												<TableHead>Clients</TableHead>
												<TableHead className="text-right">Cost</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{usersQuery.data.users.map((user) => (
												<TableRow key={user.user_id} className="text-xs">
													<TableCell className="max-w-[200px] px-3 py-2">
														<AvatarData
															truncate
															title={user.name || user.username}
															subtitle={`@${user.username}`}
															src={user.avatar_url}
															imgFallbackText={user.username}
														/>
													</TableCell>
													<TableCell className="w-40 max-w-40">
														<ProvidersBadge providers={user.providers} />
													</TableCell>
													<TableCell className="w-40 max-w-40">
														<ClientsBadge clients={user.clients} />
													</TableCell>
													<CostCell
														costMicros={user.cost_micros}
														unpricedUsageCount={user.unpriced_usage_count}
													/>
												</TableRow>
											))}
										</TableBody>
									</Table>
								</PaginationContainer>
							</div>
						)}
					</div>
				</>
			)}
		</div>
	);
};

// The total covers every matching user in the period, not only the page.
const SpendTotal: FC<{ report: TypesGen.OrganizationAISpendReport }> = ({
	report,
}) => (
	<div className="flex flex-col gap-1">
		<span className="text-sm text-content-secondary">Total spend</span>
		<span className="text-2xl font-semibold tabular-nums text-content-primary">
			{formatCostMicros(report.totals.cost_micros)}
		</span>
		{report.totals.unpriced_usage_count > 0 && (
			<span className="flex items-center gap-1 text-sm text-content-warning">
				<TriangleAlertIcon aria-hidden className="size-icon-xs" />
				Some usage could not be priced and is not included in this total.
			</span>
		)}
	</div>
);
