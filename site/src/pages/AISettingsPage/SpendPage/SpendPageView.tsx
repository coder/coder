import { EllipsisVerticalIcon } from "lucide-react";
import type { FC } from "react";

import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import { SearchField } from "#/components/SearchField/SearchField";
import { SectionHeader } from "#/components/SectionHeader/SectionHeader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
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
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/components/TemporarySavedState/TemporarySavedState";
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
import { SpendDrillInView } from "./components/SpendDrillInView";
import { formatUsageDateRange, toInclusiveDateRange } from "./utils/dateRange";

const UserRow: FC<{
	user: TypesGen.ChatCostUserRollup;
	onSelect: (user: TypesGen.ChatCostUserRollup) => void;
	onEditBudget: (user: TypesGen.ChatCostUserRollup) => void;
}> = ({ user, onSelect, onEditBudget }) => {
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
			<TableCell className="w-1" onClick={(event) => event.stopPropagation()}>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							type="button"
							size="icon"
							variant="subtle"
							aria-label={`Open spend actions for ${user.name || user.username}`}
						>
							<EllipsisVerticalIcon aria-hidden="true" className="size-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => onEditBudget(user)}>
							Update budget
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => onSelect(user)}>
							View spend details
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

interface SpendPageViewProps {
	configData: TypesGen.ChatUsageLimitConfigResponse | undefined;
	isLoadingConfig: boolean;
	configError: Error | null;
	refetchConfig: () => void;
	groupsData: TypesGen.Group[] | undefined;
	isLoadingGroups: boolean;
	groupsError: Error | null;
	onUpdateConfig: (
		req: TypesGen.ChatUsageLimitConfig,
		options?: { onSuccess?: () => void },
	) => void;
	isUpdatingConfig: boolean;
	updateConfigError: Error | null;
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
	activeTab: "limits" | "usage";
	onActiveTabChange: (tab: "limits" | "usage") => void;
}

export const SpendPageView: FC<SpendPageViewProps> = ({
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
	activeTab,
	onActiveTabChange,
}) => {
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

	const displayDateRange = toInclusiveDateRange(dateRange, endDateIsExclusive);
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive,
	});

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
		onUpdateConfig(
			{
				spend_limit_micros: spendLimitMicros,
				period,
				updated_at: new Date().toISOString(),
			},
			{ onSuccess: showSavedState },
		);
	};

	const groupOverrides = configData?.group_overrides ?? [];
	const overrides = configData?.overrides ?? [];
	const unpricedModelCount = configData?.unpriced_model_count ?? 0;
	const groupOrganizationNames =
		groupsData?.reduce<Record<string, string | undefined>>((acc, group) => {
			acc[group.id] = group.organization_name;
			return acc;
		}, {}) ?? {};
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

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
						<div className="flex max-w-[1100px] flex-col gap-8">
							<SettingsHeader>
								<SettingsHeaderTitle>
									Spend limits and usage
								</SettingsHeaderTitle>
								<SettingsHeaderDescription>
									Configure spend limits and monitor AI usage across your
									deployment.
								</SettingsHeaderDescription>
							</SettingsHeader>

							<Tabs
								value={activeTab}
								onValueChange={(tab) => {
									if (tab === "limits" || tab === "usage") {
										onActiveTabChange(tab);
									}
								}}
							>
								<TabsList>
									<TabsTrigger value="limits">Spend limits</TabsTrigger>
									<TabsTrigger value="usage">Usage</TabsTrigger>
								</TabsList>

								<TabsContent
									value="limits"
									className="pt-8 data-[state=inactive]:hidden"
									forceMount
								>
									<div className="space-y-8">
										{isLoadingConfig ? (
											<div className="flex items-center justify-center rounded-lg border border-border-default px-6 py-10">
												<Spinner loading className="h-6 w-6" />
											</div>
										) : configError ? (
											<div className="space-y-4">
												<ErrorAlert error={configError} />
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
														isDirty,
														saveDefault,
													}) => (
														<DefaultLimitSection
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
															isSaving={isUpdatingConfig}
															isSavedVisible={isSavedVisible}
															saveDisabled={
																isUpdatingConfig || !isAmountValid || !isDirty
															}
															onSave={isDirty ? saveDefault : undefined}
															saveStatus={
																isSavedVisible ? (
																	<TemporarySavedState />
																) : updateConfigError ? (
																	getErrorMessage(
																		updateConfigError,
																		"Failed to save the default spend limit.",
																	)
																) : null
															}
														/>
													)}
												</DefaultLimitController>

												<section className="space-y-6">
													<SectionHeader
														level="section"
														variant="spacious"
														label="Group limits"
														description="Override the default limit for specific groups. The lowest group limit applies."
													/>
													<GroupLimitsSection
														hideHeader
														groupOverrides={groupOverrides}
														groupOrganizationNames={groupOrganizationNames}
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
														editingGroupOverride={
															groupCtrl.editingGroupOverride
														}
														onEditGroupOverride={(override) => {
															userCtrl.handleShowUserFormChange(false);
															groupCtrl.handleEditGroupOverride(override);
														}}
														onAddGroupOverride={
															groupCtrl.handleAddGroupOverride
														}
														onDeleteGroupOverride={onDeleteGroupOverride}
														upsertPending={isUpsertingGroupOverride}
														upsertError={upsertGroupOverrideError}
														deletePending={isDeletingGroupOverride}
														deleteError={deleteGroupOverrideError}
														groupsError={groupsError}
													/>
												</section>

												<section className="space-y-6">
													<SectionHeader
														level="section"
														variant="spacious"
														label="User overrides"
														description="Set user-specific limits. User overrides take highest priority, followed by group limits, then the default."
													/>
													<UserOverridesSection
														hideHeader
														overrides={overrides}
														showUserForm={userCtrl.showUserForm}
														onShowUserFormChange={
															userCtrl.handleShowUserFormChange
														}
														selectedUser={userCtrl.selectedUserOverride}
														onSelectedUserChange={
															userCtrl.setSelectedUserOverride
														}
														userOverrideAmount={userCtrl.userOverrideAmount}
														onUserOverrideAmountChange={
															userCtrl.setUserOverrideAmount
														}
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
												</section>
											</>
										)}
									</div>
								</TabsContent>

								<TabsContent value="usage" className="pt-8">
									<section className="space-y-6">
										<SectionHeader
											level="section"
											variant="spacious"
											label="Usage by user"
											description="Monitor AI usage and spend for users in the selected date range."
											action={
												<DateRangePicker
													value={displayDateRange}
													onChange={onDateRangeChange}
												/>
											}
										/>
										<div>
											<div className="w-full md:max-w-sm">
												<SearchField
													value={searchFilter}
													onChange={onSearchFilterChange}
													placeholder="Search by name or username"
													aria-label="Search usage by name or username"
												/>
											</div>
										</div>
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
										{usersQuery.error != null && (
											<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
												<ErrorAlert error={usersQuery.error} />
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
															<Table aria-label="User spend details">
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
																		<TableHead className="w-1">
																			Actions
																		</TableHead>
																	</TableRow>
																</TableHeader>
																<TableBody>
																	{usersQuery.data.users.map((user) => (
																		<UserRow
																			key={user.user_id}
																			user={user}
																			onSelect={onSelectUser}
																			onEditBudget={(selectedUser) => {
																				const override = overrides.find(
																					(o) =>
																						o.user_id === selectedUser.user_id,
																				) ?? {
																					user_id: selectedUser.user_id,
																					name: selectedUser.name,
																					username: selectedUser.username,
																					avatar_url: selectedUser.avatar_url,
																					spend_limit_micros: null,
																				};
																				userCtrl.handleEditUserOverride(
																					override,
																				);
																			}}
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
								</TabsContent>
							</Tabs>
						</div>
					)}
				</UserOverrideController>
			)}
		</GroupOverrideController>
	);
};
