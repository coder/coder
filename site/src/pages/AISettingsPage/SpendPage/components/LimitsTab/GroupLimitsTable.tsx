import { EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatCostMicros } from "#/utils/currency";
import type { GroupLimitOverride } from "./GroupLimitsSection";

export const GROUP_LIMITS_PAGE_SIZE = 10;

interface GroupLimitsTableProps {
	pagedOverrides: readonly GroupLimitOverride[];
	totalOverrides: number;
	clampedPage: number;
	hasPreviousPage: boolean;
	hasNextPage: boolean;
	onPageChange: (page: number) => void;
	onEditGroupOverride: (override: GroupLimitOverride) => void;
	onRequestDelete: (groupID: string) => void;
	groupOrganizationNames?: Record<string, string | undefined>;
	deletePending: boolean;
	upsertPending: boolean;
	isEditing: boolean;
	emptyMessage: string;
}

export const GroupLimitsTable: FC<GroupLimitsTableProps> = ({
	pagedOverrides,
	totalOverrides,
	clampedPage,
	hasPreviousPage,
	hasNextPage,
	onPageChange,
	onEditGroupOverride,
	onRequestDelete,
	groupOrganizationNames,
	deletePending,
	upsertPending,
	isEditing,
	emptyMessage,
}) => {
	if (totalOverrides === 0) {
		return (
			<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
				{emptyMessage}
			</div>
		);
	}

	return (
		<>
			<div className="space-y-4">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Group</TableHead>
							<TableHead>Members</TableHead>
							<TableHead>Spend limit</TableHead>
							<TableHead className="w-1">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{pagedOverrides.map((override) => {
							const orgName = groupOrganizationNames?.[override.group_id];
							const groupAvatar = (
								<AvatarData
									title={override.group_display_name || override.group_name}
									subtitle={override.group_name}
									src={override.group_avatar_url}
									imgFallbackText={override.group_name}
								/>
							);

							return (
								<TableRow key={override.group_id}>
									<TableCell>
										{orgName ? (
											<Link
												to={`/organizations/${orgName}/groups/${override.group_name}`}
												className="inline-block"
											>
												{groupAvatar}
											</Link>
										) : (
											groupAvatar
										)}
									</TableCell>
									<TableCell>{override.member_count}</TableCell>
									<TableCell>
										{override.spend_limit_micros !== null
											? formatCostMicros(override.spend_limit_micros)
											: "Unlimited"}
									</TableCell>
									<TableCell className="w-1 whitespace-nowrap text-right">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button
													variant="subtle"
													size="icon"
													type="button"
													disabled={deletePending || upsertPending}
													aria-label={`Actions for ${
														override.group_display_name || override.group_name
													}`}
												>
													<EllipsisVerticalIcon />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem
													onSelect={() => onEditGroupOverride(override)}
												>
													Update budget
												</DropdownMenuItem>
												<DropdownMenuItem
													className="text-content-destructive focus:text-content-destructive"
													disabled={isEditing}
													onSelect={() => onRequestDelete(override.group_id)}
												>
													<TrashIcon />
													Remove group limit
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</TableRow>
							);
						})}
					</TableBody>
				</Table>
				<PaginationAmount
					limit={GROUP_LIMITS_PAGE_SIZE}
					totalRecords={totalOverrides}
					currentOffsetStart={(clampedPage - 1) * GROUP_LIMITS_PAGE_SIZE + 1}
					paginationUnitLabel="groups"
					className="justify-end"
				/>
			</div>
			{totalOverrides > GROUP_LIMITS_PAGE_SIZE && (
				<PaginationWidgetBase
					currentPage={clampedPage}
					pageSize={GROUP_LIMITS_PAGE_SIZE}
					totalRecords={totalOverrides}
					onPageChange={onPageChange}
					hasPreviousPage={hasPreviousPage}
					hasNextPage={hasNextPage}
				/>
			)}
		</>
	);
};
