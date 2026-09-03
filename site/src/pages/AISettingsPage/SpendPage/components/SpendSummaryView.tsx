import { TriangleAlertIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatTokenCount } from "#/utils/analytics";
import { formatCostMicros } from "#/utils/currency";
import { paginateItems } from "#/utils/paginateItems";
import { unpricedRequestsMessage } from "./unpricedRequests";

const BREAKDOWN_PAGE_SIZE = 10;

interface SpendSummaryViewProps {
	summary: TypesGen.AIGatewaySpendUserSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
}

export const SpendSummaryView: FC<SpendSummaryViewProps> = ({
	summary,
	isLoading,
	error,
	onRetry,
}) => {
	// Page state survives summary changes; paginateItems clamps it, so a
	// narrower date range never shows an empty page and widening it again
	// returns to where the user was.
	const [modelPage, setModelPage] = useState(1);
	const [clientPage, setClientPage] = useState(1);

	if (isLoading) {
		return (
			<div
				role="status"
				aria-label="Loading spend details"
				className="flex min-h-[240px] items-center justify-center"
			>
				<Spinner size="lg" loading />
			</div>
		);
	}

	if (error != null) {
		return (
			<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
				<p className="m-0 text-sm text-content-secondary">
					{getErrorMessage(error, "Failed to load spend details.")}
				</p>
				<Button variant="outline" size="sm" type="button" onClick={onRetry}>
					Retry
				</Button>
			</div>
		);
	}

	if (!summary) {
		return null;
	}

	const models = paginateItems(
		summary.by_model,
		BREAKDOWN_PAGE_SIZE,
		modelPage,
	);
	const clients = paginateItems(
		summary.by_client,
		BREAKDOWN_PAGE_SIZE,
		clientPage,
	);

	return (
		<div className="space-y-6">
			<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
				<SummaryCard label="Total cost">
					{formatCostMicros(summary.total_cost_micros)}
				</SummaryCard>
				<SummaryCard label="Requests">
					{summary.request_count.toLocaleString("en-US")}
				</SummaryCard>
				<SummaryCard label="Sessions">
					{summary.session_count.toLocaleString("en-US")}
				</SummaryCard>
				<SummaryCard label="Input tokens">
					{formatTokenCount(summary.input_tokens)}
				</SummaryCard>
				<SummaryCard label="Output tokens">
					{formatTokenCount(summary.output_tokens)}
				</SummaryCard>
				<SummaryCard label="Cache read">
					{formatTokenCount(summary.cache_read_input_tokens)}
				</SummaryCard>
				<SummaryCard label="Cache write">
					{formatTokenCount(summary.cache_write_input_tokens)}
				</SummaryCard>
			</div>

			{summary.unpriced_request_count > 0 && (
				<div
					role="note"
					className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary"
				>
					<TriangleAlertIcon className="size-5 shrink-0 text-content-warning" />
					<span>{unpricedRequestsMessage(summary.unpriced_request_count)}</span>
				</div>
			)}

			{summary.request_count === 0 ? (
				<p className="py-12 text-center text-content-secondary">
					No AI Gateway spend for this user in the selected period.
				</p>
			) : (
				<>
					<div>
						<Table aria-label="Spend by model">
							<TableHeader>
								<TableRow>
									<TableHead>Model</TableHead>
									<TableHead>Provider</TableHead>
									<TableHead className="text-right">Cost</TableHead>
									<TableHead className="text-right">Requests</TableHead>
									<TableHead className="text-right">Input</TableHead>
									<TableHead className="text-right">Output</TableHead>
									<TableHead className="text-right">Cache read</TableHead>
									<TableHead className="text-right">Cache write</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{models.pagedItems.map((model) => (
									<TableRow
										key={`${model.provider_name}/${model.provider}/${model.model}`}
									>
										<TableCell>{model.model}</TableCell>
										<TableCell className="text-content-secondary">
											{model.provider_name || model.provider}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatCostMicros(model.total_cost_micros)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{model.request_count.toLocaleString("en-US")}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(model.input_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(model.output_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(model.cache_read_input_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(model.cache_write_input_tokens)}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						{summary.by_model.length > BREAKDOWN_PAGE_SIZE && (
							<div className="pt-4">
								<PaginationWidgetBase
									totalRecords={summary.by_model.length}
									currentPage={models.clampedPage}
									pageSize={BREAKDOWN_PAGE_SIZE}
									onPageChange={setModelPage}
									hasPreviousPage={models.hasPreviousPage}
									hasNextPage={models.hasNextPage}
								/>
							</div>
						)}
					</div>

					<div>
						<Table aria-label="Spend by client">
							<TableHeader>
								<TableRow>
									<TableHead>Client</TableHead>
									<TableHead className="text-right">Cost</TableHead>
									<TableHead className="text-right">Requests</TableHead>
									<TableHead className="text-right">Sessions</TableHead>
									<TableHead className="text-right">Input</TableHead>
									<TableHead className="text-right">Output</TableHead>
									<TableHead className="text-right">Cache read</TableHead>
									<TableHead className="text-right">Cache write</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{clients.pagedItems.map((client) => (
									<TableRow key={client.client}>
										<TableCell>{client.client}</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatCostMicros(client.total_cost_micros)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{client.request_count.toLocaleString("en-US")}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{client.session_count.toLocaleString("en-US")}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(client.input_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(client.output_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(client.cache_read_input_tokens)}
										</TableCell>
										<TableCell className="text-right tabular-nums">
											{formatTokenCount(client.cache_write_input_tokens)}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						{summary.by_client.length > BREAKDOWN_PAGE_SIZE && (
							<div className="pt-4">
								<PaginationWidgetBase
									totalRecords={summary.by_client.length}
									currentPage={clients.clampedPage}
									pageSize={BREAKDOWN_PAGE_SIZE}
									onPageChange={setClientPage}
									hasPreviousPage={clients.hasPreviousPage}
									hasNextPage={clients.hasNextPage}
								/>
							</div>
						)}
					</div>
				</>
			)}
		</div>
	);
};

const SummaryCard: FC<{ label: string; children: ReactNode }> = ({
	label,
	children,
}) => (
	<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
		<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
			{label}
		</p>
		<p className="mt-1 text-2xl font-semibold text-content-primary">
			{children}
		</p>
	</div>
);
