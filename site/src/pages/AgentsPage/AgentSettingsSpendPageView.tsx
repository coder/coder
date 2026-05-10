import type { FC } from "react";

import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import { SearchField } from "#/components/SearchField/SearchField";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { formatTokenCount } from "#/utils/analytics";
import {
	dollarsToMicros,
	formatCostMicros,
	microsToDollars,
} from "#/utils/currency";
import {
	DefaultLimitController,
	type DefaultLimitFormValues,
} from "./components/LimitsTab/DefaultLimitController";
import { DefaultLimitSection } from "./components/LimitsTab/DefaultLimitSection";
import { GroupLimitsSection } from "./components/LimitsTab/GroupLimitsSection";
import { GroupOverrideController } from "./components/LimitsTab/GroupOverrideController";
import { normalizeChatUsageLimitPeriod } from "./components/LimitsTab/limitsFormLogic";
import { UserOverrideController } from "./components/LimitsTab/UserOverrideController";
import { UserOverridesSection } from "./components/LimitsTab/UserOverridesSection";
import { SectionHeader } from "./components/SectionHeader";
import { SpendDrillInView } from "./components/SpendDrillInView";
import { formatUsageDateRange, toInclusiveDateRange } from "./utils/dateRange";

// ── UserRow sub-component ──

const UserRow: FC<{
	user: TypesGen.ChatCostUserRollup;
	onSelect: (user: TypesGen.ChatCostUserRollup) => void;
}> = ({ user, onSelect }) => {
	const clickableRowProps = useClickableTableRow({
		onClick: () => onSelect(user),
	});

	return (
		<TableRow
			{...clickableRowProps}
			aria-label={`View details for ${user.name || user.username}`}
			className="text-xs"
		>
			<TableCell className="max-w-[200px] px-3 py-2">
				<Tooltip>
					<TooltipTrigger asChild>
						<div>
							<AvatarData
								title={
									<span className="block truncate">
										{user.name || user.username}
									</span>
								}
								subtitle={
									<span className="block truncate">@{user.username}</span>
								}
								src={user.avatar_url}
								imgFallbackText={user.username}
							/>
						</div>
					</TooltipTrigger>
					<TooltipContent>{user.name || user.username}</TooltipContent>
				</Tooltip>
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatCostMicros(user.total_cost_micros)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.message_count.toLocaleString()}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.chat_count.toLocaleString()}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_input_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_output_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_cache_read_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_cache_creation_tokens)}
			</TableCell>
		</TableRow>
	);
};

// ── Props ──

interface AgentSettingsSpendPageViewProps {
	// Limits config.
	configData: TypesGen.ChatUsageLimitConfigResponse | undefined;
	isLoadingConfig: boolean;
	configError: Error | null;
	refetchConfig: () => void;
	groupsData: TypesGen.Group[] | undefined;
	isLoadingGroups: boolean;
	groupsError: Error | null;
	onUpdateConfig: (req: TypesGen.ChatUsageLimitConfig) => void;
	isUpdatingConfig: boolean;
	updateConfigError: Error | null;
	isUpdateConfigSuccess: boolean;
	resetUpdateConfig: () => void;
	onUpsertOverride: (args: {
		userID: string;
		req: TypesGen.UpsertChatUsageLimitOverrideRequest;
		onSuccess: () => void;
	}) => void;
	isUpsertingOverride: boolean;
	upsertOverrideError: Error | null;
	onDeleteOverride: (userID: string) => void;
	isDeletingOverride: boolean;
	deleteOverrideError: Error | null;
	onUpsertGroupOverride: (args: {
		groupID: string;
		req: TypesGen.UpsertChatUsageLimitGroupOverrideRequest;
		onSuccess: () => void;
	}) => void;
	isUpsertingGroupOverride: boolean;
	upsertGroupOverrideError: Error | null;
	onDeleteGroupOverride: (groupID: string) => void;
	isDeletingGroupOverride: boolean;
	deleteGroupOverrideError: Error | null;
	// Usage data.
	dateRange: DateRangeValue;
	endDateIsExclusive: boolean;
	onDateRangeChange: (value: DateRangeValue) => void;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	usersQuery: PaginationResult & {
		data: TypesGen.ChatCostUsersResponse | undefined;
		isLoading: boolean;
		isFetching: boolean;
		error: unknown;
		refetch: () => unknown;
	};
	drillInUserId: string | null;
	drillInUser: TypesGen.User | null;
	isDrillInUserLoading: boolean;
	isDrillInUserError: boolean;
	drillInUserError: unknown;
	onDrillInUserRetry: () => void;
	onClearSelectedUser: () => void;
	onSelectUser: (user: TypesGen.ChatCostUserRollup) => void;
	summaryData: TypesGen.ChatCostSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

// ── View component ──

export const AgentSettingsSpendPageView: FC<
	AgentSettingsSpendPageViewProps
> = ({
	configData,
	isLoadingConfig,
	configError,
	refetchConfig,
	groupsData,
	isLoadingGroups,
	groupsError,
	onUpdateConfig,
	isUpdatingConfig,
	updateConfigError,
	isUpdateConfigSuccess,
	resetUpdateConfig,
	onUpsertOverride,
	isUpsertingOverride,
	upsertOverrideError,
	onDeleteOverride,
	isDeletingOverride,
	deleteOverrideError,
	onUpsertGroupOverride,
	isUpsertingGroupOverride,
	upsertGroupOverrideError,
	onDeleteGroupOverride,
	isDeletingGroupOverride,
	deleteGroupOverrideError,
	dateRange,
	endDateIsExclusive,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	usersQuery,
	drillInUserId,
	drillInUser,
	isDrillInUserLoading,
	isDrillInUserError,
	drillInUserError,
	onDrillInUserRetry,
	onClearSelectedUser,
	onSelectUser,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	// ── Derived limit values ──
	const defaultLimitValues: DefaultLimitFormValues = (() => {
		const spendLimitMicros = configData?.spend_limit_micros;
		const enabled = spendLimitMicros !== null && spendLimitMicros !== undefined;

		return {
			enabled,
			period: normalizeChatUsageLimitPeriod(configData?.period),
			amountDollars: enabled
				? microsToDollars(spendLimitMicros).toString()
				: "",
		};
	})();
	const defaultLimitKey = JSON.stringify({
		spend_limit_micros: configData?.spend_limit_micros ?? null,
		period: defaultLimitValues.period,
	});

	// ── Derived usage display state ──
	const displayDateRange = toInclusiveDateRange(dateRange, endDateIsExclusive);
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive,
	});

	// ── Limits handlers ──
	const handleResetUpdateConfig = () => {
		if (!isUpdatingConfig) {
			resetUpdateConfig();
		}
	};

	const handleSaveDefault = ({
		enabled,
		period,
		amountDollars,
	}: DefaultLimitFormValues) => {
		const spendLimitMicros = enabled ? dollarsToMicros(amountDollars) : null;
		onUpdateConfig({
			spend_limit_micros: spendLimitMicros,
			period,
			updated_at: new Date().toISOString(),
		});
	};

	const groupOverrides = configData?.group_overrides ?? [];
	const overrides = configData?.overrides ?? [];
	const unpricedModelCount = configData?.unpriced_model_count ?? 0;

	if (drillInUserId) {
		return (
			<SpendDrillInView
				selectedUser={drillInUser}
				isLoading={isDrillInUserLoading}
				isError={isDrillInUserError}
				error={drillInUserError}
				onRetry={onDrillInUserRetry}
				onBack={onClearSelectedUser}
				displayDateRange={displayDateRange}
				onDateRangeChange={onDateRangeChange}
				dateRangeLabel={dateRangeLabel}
				summaryData={summaryData}
				isSummaryLoading={isSummaryLoading}
				summaryError={summaryError}
				onSummaryRetry={onSummaryRetry}
			/>
		);
	}
	// ── List mode ──
	return (
		<GroupOverrideController
			groupOverrides={groupOverrides}
			groups={groupsData ?? []}
			isLoadingGroups={isLoadingGroups}
			onUpsertGroupOverride={onUpsertGroupOverride}
		>
			{(groupCtrl) => (
				<UserOverrideController
					overrides={overrides}
					onUpsertOverride={onUpsertOverride}
				>
					{(userCtrl) => (
						<div className="space-y-10">
							<SectionHeader
								label="Spend management"
								description="Configure spend limits and monitor usage across your deployment."
							/>

							{isLoadingConfig ? (
								<div className="flex items-center justify-center rounded-lg border border-border-default px-6 py-10">
									<Spinner loading className="h-6 w-6" />
								</div>
							) : configError ? (
								<div className="flex flex-col items-center justify-center gap-4 rounded-lg border border-border-default px-6 py-10 text-center">
									<p className="m-0 text-sm text-content-secondary">
										{getErrorMessage(
											configError,
											"Failed to load spend limit settings.",
										)}
									</p>
									<Button
										variant="outline"
										size="sm"
										type="button"
										onClick={() => void refetchConfig()}
									>
										Retry
									</Button>
								</div>
							) : (
								<>
									{/* Section 1: Default spend limit */}
									<DefaultLimitController
										key={defaultLimitKey}
										initialValues={defaultLimitValues}
										onSave={handleSaveDefault}
									>
										{({
											enabled,
											onEnabledChange,
											period,
											onPeriodChange,
											amountDollars,
											onAmountDollarsChange,
											isAmountValid,
											saveDefault,
										}) => (
											<section>
												<SectionHeader
													level="section"
													label="Default spend limit"
													description="Set a deployment-wide spend cap that applies to all users by default."
												/>
												<DefaultLimitSection
													hideHeader
													adminBadge={null}
													enabled={enabled}
													onEnabledChange={(v) => {
														handleResetUpdateConfig();
														onEnabledChange(v);
													}}
													period={period}
													onPeriodChange={(v) => {
														handleResetUpdateConfig();
														onPeriodChange(v);
													}}
													amountDollars={amountDollars}
													onAmountDollarsChange={(v) => {
														handleResetUpdateConfig();
														onAmountDollarsChange(v);
													}}
													unpricedModelCount={unpricedModelCount}
												/>
												<div className="flex items-center justify-end gap-3 pt-4">
													<div className="min-h-4 text-xs">
														{updateConfigError && (
															<p className="m-0 text-content-destructive">
																{getErrorMessage(
																	updateConfigError,
																	"Failed to save the default spend limit.",
																)}
															</p>
														)}
														{isUpdateConfigSuccess && (
															<p className="m-0 text-content-success">Saved!</p>
														)}
													</div>
													<Button
														size="sm"
														type="button"
														onClick={saveDefault}
														disabled={isUpdatingConfig || !isAmountValid}
													>
														{isUpdatingConfig ? (
															<Spinner loading className="h-4 w-4" />
														) : null}
														Save default limit
													</Button>
												</div>
											</section>
										)}
									</DefaultLimitController>

									{/* Section 2: Group limits */}
									<section>
										<SectionHeader
											level="section"
											label="Group limits"
											description="Override the default limit for specific groups. The lowest group limit applies."
										/>
										<GroupLimitsSection
											hideHeader
											groupOverrides={groupOverrides}
											groupOrganizationNames={groupCtrl.groupOrganizationNames}
											showGroupForm={groupCtrl.showGroupForm}
											onShowGroupFormChange={
												groupCtrl.handleShowGroupFormChange
											}
											selectedGroup={groupCtrl.selectedGroup}
											onSelectedGroupChange={groupCtrl.setSelectedGroup}
											groupAmount={groupCtrl.groupAmount}
											onGroupAmountChange={groupCtrl.setGroupAmount}
											availableGroups={groupCtrl.availableGroups}
											groupAutocompleteNoOptionsText={
												groupCtrl.groupAutocompleteNoOptionsText
											}
											groupsLoading={isLoadingGroups}
											editingGroupOverride={groupCtrl.editingGroupOverride}
											onEditGroupOverride={(override) => {
												userCtrl.handleShowUserFormChange(false);
												groupCtrl.handleEditGroupOverride(override);
											}}
											onAddGroupOverride={groupCtrl.handleAddGroupOverride}
											onDeleteGroupOverride={onDeleteGroupOverride}
											upsertPending={isUpsertingGroupOverride}
											upsertError={upsertGroupOverrideError}
											deletePending={isDeletingGroupOverride}
											deleteError={deleteGroupOverrideError}
											groupsError={groupsError}
										/>
									</section>
								</>
							)}

							{/* Section 3: Per-user spend */}
							<section>
								<SectionHeader
									level="section"
									label="Per-user spend"
									description="User overrides take highest priority, followed by group limits, then the default."
								/>
								<div className="flex items-center justify-between pb-4">
									<span className="text-sm font-medium text-content-primary">
										Date range
									</span>
									<DateRangePicker
										value={displayDateRange}
										onChange={onDateRangeChange}
									/>
								</div>
								{!configError && !isLoadingConfig && (
									<UserOverridesSection
										hideHeader
										overrides={overrides}
										showUserForm={userCtrl.showUserForm}
										onShowUserFormChange={userCtrl.handleShowUserFormChange}
										selectedUser={userCtrl.selectedUserOverride}
										onSelectedUserChange={userCtrl.setSelectedUserOverride}
										userOverrideAmount={userCtrl.userOverrideAmount}
										onUserOverrideAmountChange={userCtrl.setUserOverrideAmount}
										selectedUserAlreadyOverridden={
											userCtrl.editingUserOverride
												? false
												: userCtrl.selectedUserAlreadyOverridden
										}
										editingUserOverride={userCtrl.editingUserOverride}
										onEditUserOverride={(override) => {
											groupCtrl.handleShowGroupFormChange(false);
											userCtrl.handleEditUserOverride(override);
										}}
										onAddOverride={userCtrl.handleAddOverride}
										onDeleteOverride={onDeleteOverride}
										upsertPending={isUpsertingOverride}
										upsertError={upsertOverrideError}
										deletePending={isDeletingOverride}
										deleteError={deleteOverrideError}
									/>
								)}
								{/* Search */}
								<div className="pt-6">
									<div className="w-full md:max-w-sm">
										<SearchField
											value={searchFilter}
											onChange={onSearchFilterChange}
											placeholder="Search by name or username"
											aria-label="Search usage by name or username"
										/>
									</div>
								</div>
								{/* Loading state */}
								{usersQuery.isLoading && (
									<div
										role="status"
										aria-label="Loading usage"
										className="flex min-h-[240px] items-center justify-center"
									>
										<Spinner
											size="lg"
											loading
											className="text-content-secondary"
										/>
									</div>
								)}
								{/* Error state */}
								{usersQuery.error != null && (
									<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
										<p className="m-0 text-sm text-content-secondary">
											{getErrorMessage(
												usersQuery.error,
												"Failed to load usage data.",
											)}
										</p>
										<Button
											variant="outline"
											size="sm"
											type="button"
											onClick={() => void usersQuery.refetch()}
										>
											Retry
										</Button>
									</div>
								)}
								{/* User table + pagination */}
								{usersQuery.data && (
									<div className="relative pt-3">
										{usersQuery.isFetching && !usersQuery.isLoading && (
											<div
												role="status"
												aria-label="Refreshing usage"
												className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
											>
												<Spinner
													size="lg"
													loading
													className="text-content-secondary"
												/>
											</div>
										)}
										{usersQuery.data.users.length === 0 ? (
											<p className="py-12 text-center text-content-secondary">
												No usage data for this period.
											</p>
										) : (
											<PaginationContainer
												query={usersQuery}
												paginationUnitLabel="users"
											>
												<div className="overflow-hidden rounded-lg border border-border-default">
													<Table aria-label="Per-user spend">
														<TableHeader>
															<TableRow>
																<TableHead>User</TableHead>
																<TableHead className="text-right">
																	Cost
																</TableHead>
																<TableHead className="text-right">
																	Messages
																</TableHead>
																<TableHead className="text-right">
																	Chats
																</TableHead>
																<TableHead className="text-right">
																	Input
																</TableHead>
																<TableHead className="text-right">
																	Output
																</TableHead>
																<TableHead className="text-right">
																	Cache Read
																</TableHead>
																<TableHead className="text-right">
																	Cache Write
																</TableHead>
															</TableRow>
														</TableHeader>
														<TableBody>
															{usersQuery.data.users.map((user) => (
																<UserRow
																	key={user.user_id}
																	user={user}
																	onSelect={onSelectUser}
																/>
															))}
														</TableBody>
													</Table>
												</div>
											</PaginationContainer>
										)}
									</div>
								)}
							</section>
						</div>
					)}
				</UserOverrideController>
			)}
		</GroupOverrideController>
	);
};
