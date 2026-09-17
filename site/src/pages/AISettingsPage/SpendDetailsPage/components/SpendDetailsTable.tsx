import type { FC } from "react";
import type { OrganizationAISpendRow } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
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

interface SpendDetailsTableProps {
	rows: readonly OrganizationAISpendRow[] | undefined;
	isLoading: boolean;
}

export const SpendDetailsTable: FC<SpendDetailsTableProps> = ({
	rows,
	isLoading,
}) => (
	<Table aria-label="AI spend details" className="min-w-[900px] text-sm">
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
			{isLoading ? (
				<TableLoader />
			) : rows?.length === 0 ? (
				<TableEmpty message="No AI Gateway spend matches these filters." />
			) : (
				rows?.map((row) => (
					<TableRow
						key={`${row.user_id}:${row.group_id}:${row.provider}:${row.provider_name}:${row.model}`}
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
								<AIBridgeModelIcon model={row.model} className="size-icon-xs" />
								<span className="truncate min-w-0" title={row.model || "N/A"}>
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
);
