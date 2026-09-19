import type { FC } from "react";
import type { OrganizationAISpendDetails } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
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
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/icons/AIBridgeProviderIcon";
import { TokenBadges } from "#/pages/AIBridgePage/TokenBadges";
import { formatCostMicros } from "#/utils/currency";

export type SpendDetailsQuery = PaginationResult<OrganizationAISpendDetails> & {
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
};

export const SpendDetailsTable: FC<{ query: SpendDetailsQuery }> = ({
	query,
}) => {
	const retryButton = (
		<Button variant="outline" size="sm" onClick={() => void query.refetch()}>
			Retry
		</Button>
	);

	if (query.error && !query.data) {
		return (
			<div className="flex min-h-[240px] flex-col items-center justify-center gap-4">
				<ErrorAlert error={query.error} />
				{retryButton}
			</div>
		);
	}

	return (
		<div className="space-y-4">
			{query.error != null && query.data && (
				<ErrorAlert error={query.error} actions={retryButton} />
			)}
			<PaginationContainer query={query} paginationUnitLabel="rows">
				<div className="relative">
					{query.isFetching && !query.isLoading && (
						<div
							role="status"
							aria-label="Refreshing spend details"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					<Table
						aria-label="AI spend details"
						className="min-w-[900px] text-sm"
					>
						<TableHeader>
							<TableRow>
								<TableHead>User</TableHead>
								<TableHead>Group</TableHead>
								<TableHead>Provider</TableHead>
								<TableHead>Model</TableHead>
								<TableHead>Spend</TableHead>
								<TableHead className="text-nowrap">Input/Output</TableHead>
								<TableHead className="text-nowrap">Cache read/write</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{query.isLoading ? (
								<TableLoader />
							) : query.data?.rows.length === 0 ? (
								<TableEmpty message="No AI Gateway spend matches these filters." />
							) : (
								query.data?.rows.map((row) => (
									<TableRow
										key={JSON.stringify([
											row.user_id,
											row.group_id,
											row.provider,
											row.provider_name,
											row.model,
										])}
									>
										<TableCell>{row.username || row.user_id}</TableCell>
										<TableCell>{row.group_name || row.group_id}</TableCell>
										<TableCell className="max-w-48">
											<Badge className="gap-1.5 max-w-full min-w-0 overflow-hidden">
												<AIBridgeProviderIcon
													provider={row.provider}
													className="size-icon-xs"
												/>
												<span
													className="truncate min-w-0"
													title={row.provider_name || row.provider || "N/A"}
												>
													{row.provider_name || row.provider || "N/A"}
												</span>
											</Badge>
										</TableCell>
										<TableCell className="max-w-60">
											<Badge className="gap-1.5 max-w-full min-w-0 overflow-hidden">
												<AIBridgeModelIcon
													model={row.model}
													className="size-icon-xs"
												/>
												<span
													className="truncate min-w-0"
													title={row.model || "N/A"}
												>
													{row.model || "N/A"}
												</span>
											</Badge>
										</TableCell>
										<TableCell className="tabular-nums">
											{formatCostMicros(row.cost_micros)}
										</TableCell>
										<TableCell className="w-40">
											<TokenBadges
												inputTokens={row.input_tokens}
												outputTokens={row.output_tokens}
											/>
										</TableCell>
										<TableCell className="w-40">
											<TokenBadges
												inputTokens={row.cache_read_tokens}
												outputTokens={row.cache_write_tokens}
												inputLabel="Cache read"
												outputLabel="Cache write"
											/>
										</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				</div>
			</PaginationContainer>
		</div>
	);
};
