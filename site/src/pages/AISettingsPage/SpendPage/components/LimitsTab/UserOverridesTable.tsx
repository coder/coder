import { EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
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
import type { UserOverride } from "./UserOverridesSection";

export const USER_OVERRIDES_PAGE_SIZE = 10;

interface UserOverridesTableProps {
	pagedOverrides: readonly UserOverride[];
	totalOverrides: number;
	clampedPage: number;
	hasPreviousPage: boolean;
	hasNextPage: boolean;
	onPageChange: (page: number) => void;
	onEditUserOverride: (override: UserOverride) => void;
	onRequestDelete: (userID: string) => void;
	deletePending: boolean;
	upsertPending: boolean;
	isEditing: boolean;
	emptyMessage: string;
}

export const UserOverridesTable: FC<UserOverridesTableProps> = ({
	pagedOverrides,
	totalOverrides,
	clampedPage,
	hasPreviousPage,
	hasNextPage,
	onPageChange,
	onEditUserOverride,
	onRequestDelete,
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
							<TableHead>User</TableHead>
							<TableHead>Spend limit</TableHead>
							<TableHead className="w-1">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{pagedOverrides.map((override) => (
							<TableRow key={override.user_id}>
								<TableCell>
									<AvatarData
										title={override.name || override.username}
										subtitle={`@${override.username}`}
										src={override.avatar_url}
										imgFallbackText={override.username}
									/>
								</TableCell>
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
												aria-label={`Actions for ${override.name || override.username}`}
											>
												<EllipsisVerticalIcon />
											</Button>
										</DropdownMenuTrigger>
										<DropdownMenuContent align="end">
											<DropdownMenuItem
												onSelect={() => onEditUserOverride(override)}
											>
												Update budget
											</DropdownMenuItem>
											<DropdownMenuItem
												className="text-content-destructive focus:text-content-destructive"
												disabled={isEditing}
												onSelect={() => onRequestDelete(override.user_id)}
											>
												<TrashIcon />
												Remove override
											</DropdownMenuItem>
										</DropdownMenuContent>
									</DropdownMenu>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
				<PaginationAmount
					limit={USER_OVERRIDES_PAGE_SIZE}
					totalRecords={totalOverrides}
					currentOffsetStart={(clampedPage - 1) * USER_OVERRIDES_PAGE_SIZE + 1}
					paginationUnitLabel="users"
					className="justify-end"
				/>
			</div>
			{totalOverrides > USER_OVERRIDES_PAGE_SIZE && (
				<PaginationWidgetBase
					currentPage={clampedPage}
					pageSize={USER_OVERRIDES_PAGE_SIZE}
					totalRecords={totalOverrides}
					onPageChange={onPageChange}
					hasPreviousPage={hasPreviousPage}
					hasNextPage={hasNextPage}
				/>
			)}
		</>
	);
};
