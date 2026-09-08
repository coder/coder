import { cn } from "cn";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { AgentTimeReport } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	type AgentTimeSortBy,
	type AgentTimeSortOrder,
	type AgentTimeTableGroup,
	entityHeading,
	formatAgentTimeHours,
	formatShare,
	rowName,
} from "./agentTimeUtils";

function SortIcon({
	active,
	order,
}: {
	active: boolean;
	order: AgentTimeSortOrder;
}) {
	if (!active) {
		return null;
	}
	return order === "asc" ? (
		<ChevronUpIcon className="size-3.5" />
	) : (
		<ChevronDownIcon className="size-3.5" />
	);
}

function SortHeader({
	children,
	column,
	sortBy,
	sortOrder,
	onSortChange,
	className,
}: {
	children: ReactNode;
	column: AgentTimeSortBy;
	sortBy: AgentTimeSortBy;
	sortOrder: AgentTimeSortOrder;
	onSortChange: (sortBy: AgentTimeSortBy) => void;
	className?: string;
}) {
	const active = sortBy === column;
	return (
		<TableHead
			className={className}
			aria-sort={
				active ? (sortOrder === "asc" ? "ascending" : "descending") : "none"
			}
		>
			<button
				type="button"
				className={cn(
					"inline-flex items-center gap-1 border-0 bg-transparent p-0 text-left font-semibold text-inherit",
					"cursor-pointer hover:text-content-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link",
					className?.includes("text-right") && "justify-end",
				)}
				onClick={() => onSortChange(column)}
			>
				{children}
				<SortIcon active={active} order={sortOrder} />
			</button>
		</TableHead>
	);
}

export function BreakdownTable({
	query,
	tableGroup,
	totalAgentTimeMs,
	sortBy,
	sortOrder,
	selectedUserId,
	onSortChange,
	onSelectOrganization,
	onSelectUser,
}: {
	query: PaginationResult<AgentTimeReport>;
	tableGroup: AgentTimeTableGroup;
	totalAgentTimeMs: string;
	sortBy: AgentTimeSortBy;
	sortOrder: AgentTimeSortOrder;
	selectedUserId?: string;
	onSortChange: (sortBy: AgentTimeSortBy) => void;
	onSelectOrganization: (organizationId: string) => void;
	onSelectUser: (userId: string) => void;
}) {
	const rows = query.data?.rows ?? [];
	return (
		<PaginationContainer
			query={query}
			paginationUnitLabel={entityHeading(tableGroup).toLowerCase()}
		>
			<p className="text-xs text-content-secondary sm:hidden">
				Scroll the table horizontally to see all columns.
			</p>
			<ScrollArea
				orientation="horizontal"
				viewportAriaLabel={`${entityHeading(tableGroup)} table, scroll horizontally`}
				viewportTabIndex={0}
				viewportClassName="focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link"
			>
				<Table
					wrapperClassName="overflow-visible"
					className="min-w-[600px]"
					aria-label={`${entityHeading(tableGroup)} agent time`}
				>
					<TableHeader>
						<TableRow>
							<SortHeader
								column="name"
								sortBy={sortBy}
								sortOrder={sortOrder}
								onSortChange={onSortChange}
							>
								Name
							</SortHeader>
							<SortHeader
								column="agent_time"
								sortBy={sortBy}
								sortOrder={sortOrder}
								onSortChange={onSortChange}
								className="text-right"
							>
								Agent time
							</SortHeader>
							<TableHead className="text-right">Share</TableHead>
							<TableHead>Status</TableHead>
							<TableHead className="text-right">Action</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{rows.map((row) => {
							const name = rowName(row, tableGroup);
							const isSelectedUser =
								tableGroup === "user" && row.id === selectedUserId;
							return (
								<TableRow key={row.id}>
									<TableCell>
										<div className="flex min-w-0 flex-col gap-1">
											<div className="flex min-w-0 items-center gap-2">
												<span className="truncate font-medium text-content-primary">
													{name}
												</span>
												{row.deleted && (
													<Badge variant="warning" size="sm">
														Deleted
													</Badge>
												)}
											</div>
											<span className="truncate font-mono text-xs text-content-disabled">
												{row.id}
											</span>
										</div>
									</TableCell>
									<TableCell className="text-right tabular-nums text-content-primary">
										{formatAgentTimeHours(row.agent_time_ms)}
									</TableCell>
									<TableCell className="text-right tabular-nums">
										{formatShare(row.agent_time_ms, totalAgentTimeMs)}
									</TableCell>
									<TableCell>{row.deleted ? "Deleted" : "Active"}</TableCell>
									<TableCell className="text-right">
										{tableGroup === "organization" ? (
											<Button
												variant="subtle"
												size="sm"
												type="button"
												onClick={() => onSelectOrganization(row.id)}
											>
												View users
											</Button>
										) : (
											<Button
												variant={isSelectedUser ? "outline" : "subtle"}
												size="sm"
												type="button"
												onClick={() => onSelectUser(row.id)}
												disabled={isSelectedUser}
											>
												{isSelectedUser ? "Selected" : "View user"}
											</Button>
										)}
									</TableCell>
								</TableRow>
							);
						})}
					</TableBody>
				</Table>
			</ScrollArea>
		</PaginationContainer>
	);
}
