import type { FC } from "react";
import type { UseQueryResult } from "react-query";
import type { To } from "react-router";
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
import { SpendSummary } from "./SpendSummary";

export type SpendReportQuery =
	PaginationResult<TypesGen.OrganizationAISpendReport> &
		Pick<
			UseQueryResult<TypesGen.OrganizationAISpendReport, unknown>,
			"isLoading" | "isFetching" | "error" | "refetch"
		>;

/** Which models lack pricing, for the spend warnings. */
export type UnpricedModelsInfo = {
	/** Undefined when the user's unpriced models cannot be determined. */
	forUser: (
		user: TypesGen.OrganizationAISpendUser,
	) => readonly string[] | undefined;
	/** Every matching user's unpriced models, or undefined if unknown. */
	total: readonly string[] | undefined;
	/** Where admins set model pricing; undefined for everyone else. */
	setPricingHref: To | undefined;
};

type SpendUsersTableProps = {
	reportQuery: SpendReportQuery;
	period: { start: Date; end: Date };
	unpricedModels: UnpricedModelsInfo;
};

export const SpendUsersTable: FC<SpendUsersTableProps> = ({
	reportQuery,
	period,
	unpricedModels,
}) => {
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
				<SpendSummary
					report={report}
					period={period}
					unpricedModels={unpricedModels.total}
					setPricingHref={unpricedModels.setPricingHref}
				/>
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
									<SpendUserRow
										key={user.user_id}
										user={user}
										unpricedModels={unpricedModels}
									/>
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
	unpricedModels: UnpricedModelsInfo;
};

const SpendUserRow: FC<SpendUserRowProps> = ({ user, unpricedModels }) => (
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
				user={user.name || user.username}
				costMicros={user.cost_micros}
				unpricedUsageCount={user.unpriced_usage_count}
				unpricedModels={
					user.unpriced_usage_count > 0
						? unpricedModels.forUser(user)
						: undefined
				}
				setPricingHref={unpricedModels.setPricingHref}
			/>
		</TableCell>
	</TableRow>
);
