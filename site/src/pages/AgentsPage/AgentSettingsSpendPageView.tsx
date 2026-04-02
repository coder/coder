import { ChevronLeftIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";

import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatUsageLimitPeriod, Group, User } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
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
	isPositiveFiniteDollarAmount,
	microsToDollars,
} from "#/utils/currency";
import { AdminBadge } from "./components/AdminBadge";
import { ChatCostSummaryView } from "./components/ChatCostSummaryView";
import { CollapsibleSection } from "./components/CollapsibleSection";
import { DefaultLimitSection } from "./components/LimitsTab/DefaultLimitSection";
import { GroupLimitsSection } from "./components/LimitsTab/GroupLimitsSection";
import { normalizeChatUsageLimitPeriod } from "./components/LimitsTab/limitsFormLogic";
import { UserOverridesSection } from "./components/LimitsTab/UserOverridesSection";
import { SectionHeader } from "./components/SectionHeader";
import { formatUsageDateRange, toInclusiveDateRange } from "./utils/dateRange";

// ── Local render-prop controller for the default limit form ──

interface DefaultLimitFormValues {
	enabled: boolean;
	period: ChatUsageLimitPeriod;
	amountDollars: string;
}

interface DefaultLimitControllerProps {
	initialValues: DefaultLimitFormValues;
	onSave: (values: DefaultLimitFormValues) => void;
	children: (props: {
		enabled: boolean;
		onEnabledChange: (enabled: boolean) => void;
		period: ChatUsageLimitPeriod;
		onPeriodChange: (period: ChatUsageLimitPeriod) => void;
		amountDollars: string;
		onAmountDollarsChange: (amount: string) => void;
		isAmountValid: boolean;
		saveDefault: () => void;
	}) => ReactNode;
}

const DefaultLimitController: FC<DefaultLimitControllerProps> = ({
	initialValues,
	onSave,
	children,
}) => {
	const [enabled, setEnabled] = useState(initialValues.enabled);
	const [period, setPeriod] = useState<ChatUsageLimitPeriod>(
		initialValues.period,
	);
	const [amountDollars, setAmountDollars] = useState(
		initialValues.amountDollars,
	);
	const isAmountValid = !enabled || isPositiveFiniteDollarAmount(amountDollars);

	const handleSave = () => {
		if (enabled && !isPositiveFiniteDollarAmount(amountDollars)) {
			return;
		}

		onSave({ enabled, period, amountDollars });
	};

	return children({
		enabled,
		onEnabledChange: setEnabled,
		period,
		onPeriodChange: setPeriod,
		amountDollars,
		onAmountDollarsChange: setAmountDollars,
		isAmountValid,
		saveDefault: handleSave,
	});
};

// ── Drill-in view for a single user ──

interface SpendDrillInViewProps {
	selectedUser: TypesGen.User | null;
	isLoading: boolean;
	isError: boolean;
	error: unknown;
	onRetry: () => void;
	onBack: () => void;
	displayDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	dateRangeLabel: string;
	summaryData: TypesGen.ChatCostSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

const SpendDrillInView: FC<SpendDrillInViewProps> = ({
	selectedUser,
	isLoading,
	isError,
	error,
	onRetry,
	onBack,
	displayDateRange,
	onDateRangeChange,
	dateRangeLabel,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	const backButton = (
		<button
			type="button"
			onClick={onBack}
			className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
		>
			<ChevronLeftIcon className="h-4 w-4" />
			Back
		</button>
	);

	const header = (
		<SectionHeader
			label="Spend management"
			description="Review deployment Coder Agents usage for a specific user."
			badge={<AdminBadge />}
			action={
				<DateRangePicker
					value={displayDateRange}
					onChange={onDateRangeChange}
				/>
			}
		/>
	);

	if (isLoading) {
		return (
			<div className="space-y-6">
				<div>
					{backButton}
					{header}
				</div>
				<div className="flex min-h-[240px] items-center justify-center">
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			</div>
		);
	}

	if (isError || !selectedUser) {
		return (
			<div className="space-y-6">
				<div>
					{backButton}
					{header}
				</div>
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{getErrorMessage(error, "Failed to load user profile.")}
					</p>
					<Button variant="outline" size="sm" type="button" onClick={onRetry}>
						Retry
					</Button>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div>
				{backButton}
				{header}
			</div>
			<div className="flex flex-wrap items-center gap-3 rounded-lg border border-border-default bg-surface-secondary px-4 py-3">
				<AvatarData
					title={selectedUser.name || selectedUser.username}
					subtitle={`@${selectedUser.username}`}
					src={selectedUser.avatar_url}
					imgFallbackText={selectedUser.username}
				/>
				<div className="min-w-0 text-xs text-content-secondary">
					<div>User ID: {selectedUser.id}</div>
					<div>{dateRangeLabel}</div>
				</div>
			</div>
			<ChatCostSummaryView
				key={selectedUser.id}
				summary={summaryData}
				isLoading={isSummaryLoading}
				error={summaryError}
				onRetry={onSummaryRetry}
				loadingLabel="Loading usage details"
				emptyMessage="No usage data for this user in the selected period."
			/>
		</div>
	);
};

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
	hasExplicitDateRange: boolean;
	onDateRangeChange: (value: DateRangeValue) => void;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	page: number;
	onPageChange: (page: number) => void;
	pageSize: number;
	offset: number;
	usersData: TypesGen.ChatCostUsersResponse | undefined;
	isUsersLoading: boolean;
	isUsersFetching: boolean;
	usersError: unknown;
	onUsersRetry: () => void;
	selectedUserId: string | null;
	selectedUser: TypesGen.User | null;
	isSelectedUserLoading: boolean;
	isSelectedUserError: boolean;
	selectedUserError: unknown;
	onSelectedUserRetry: () => void;
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
	hasExplicitDateRange,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	page,
	onPageChange,
	pageSize,
	offset,
	usersData,
	isUsersLoading,
	isUsersFetching,
	usersError,
	onUsersRetry,
	selectedUserId,
	selectedUser,
	isSelectedUserLoading,
	isSelectedUserError,
	selectedUserError,
	onSelectedUserRetry,
	onClearSelectedUser,
	onSelectUser,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	// ── Limits form state ──
	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
	const [groupAmount, setGroupAmount] = useState("");
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedUserOverride, setSelectedUserOverride] = useState<User | null>(
		null,
	);
	const [userOverrideAmount, setUserOverrideAmount] = useState("");
	const [editingUserOverride, setEditingUserOverride] = useState<{
		user_id: string;
		name: string;
		username: string;
		avatar_url: string;
	} | null>(null);
	const [editingGroupOverride, setEditingGroupOverride] = useState<{
		group_id: string;
		group_display_name: string;
		group_name: string;
		group_avatar_url: string;
		member_count: number;
	} | null>(null);

	// ── Derived limit values ──
	const defaultLimitValues: DefaultLimitFormValues = (() => {
		const spendLimitMicros = configData?.spend_limit_micros;
		const enabled = spendLimitMicros !== null && spendLimitMicros !== undefined;

		return {
			enabled,
			period: normalizeChatUsageLimitPeriod(configData?.period),
			amountDollars:
				enabled && spendLimitMicros !== null && spendLimitMicros !== undefined
					? microsToDollars(spendLimitMicros).toString()
					: "",
		};
	})();
	const defaultLimitKey = JSON.stringify({
		spend_limit_micros: configData?.spend_limit_micros ?? null,
		period: defaultLimitValues.period,
	});
	const existingGroupIds = new Set(
		(configData?.group_overrides ?? []).map((g) => g.group_id),
	);
	const existingUserIds = new Set(
		(configData?.overrides ?? []).map((o) => o.user_id),
	);
	const availableGroups = (groupsData ?? []).filter(
		(g) => !existingGroupIds.has(g.id),
	);
	const selectedUserAlreadyOverridden = selectedUserOverride
		? existingUserIds.has(selectedUserOverride.id)
		: false;
	const groupAutocompleteNoOptionsText = isLoadingGroups
		? "Loading groups..."
		: (groupsData?.length ?? 0) === 0
			? "No groups configured"
			: availableGroups.length === 0
				? "All groups already have overrides"
				: "No groups available";

	// ── Derived usage display state ──
	const displayDateRange = toInclusiveDateRange(
		dateRange,
		hasExplicitDateRange,
	);
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive: hasExplicitDateRange,
	});
	const totalCount = usersData?.count ?? 0;
	const hasPreviousPage = page > 1;
	const hasNextPage = offset + pageSize < totalCount;

	// ── Limits handlers ──
	const handleResetUpdateConfig = () => {
		if (!isUpdatingConfig) {
			resetUpdateConfig();
		}
	};

	const handleShowUserFormChange = (show: boolean) => {
		setShowUserForm(show);
		if (!show) {
			setEditingUserOverride(null);
		}
	};

	const handleShowGroupFormChange = (show: boolean) => {
		setShowGroupForm(show);
		if (!show) {
			setEditingGroupOverride(null);
		}
	};

	const handleEditUserOverride = (override: {
		user_id: string;
		name: string;
		username: string;
		avatar_url: string;
		spend_limit_micros: number | null;
	}) => {
		setShowGroupForm(false);
		setEditingGroupOverride(null);
		setEditingUserOverride({
			user_id: override.user_id,
			name: override.name,
			username: override.username,
			avatar_url: override.avatar_url,
		});
		setSelectedUserOverride(null);
		setUserOverrideAmount(
			override.spend_limit_micros !== null
				? microsToDollars(override.spend_limit_micros).toString()
				: "",
		);
		setShowUserForm(true);
	};

	const handleEditGroupOverride = (override: {
		group_id: string;
		group_display_name: string;
		group_name: string;
		group_avatar_url: string;
		member_count: number;
		spend_limit_micros: number | null;
	}) => {
		setShowUserForm(false);
		setEditingUserOverride(null);
		setEditingGroupOverride({
			group_id: override.group_id,
			group_display_name: override.group_display_name,
			group_name: override.group_name,
			group_avatar_url: override.group_avatar_url,
			member_count: override.member_count,
		});
		setSelectedGroup(null);
		setGroupAmount(
			override.spend_limit_micros !== null
				? microsToDollars(override.spend_limit_micros).toString()
				: "",
		);
		setShowGroupForm(true);
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

	const handleAddOverride = () => {
		const targetUserID =
			editingUserOverride?.user_id ?? selectedUserOverride?.id;

		if (!targetUserID || !isPositiveFiniteDollarAmount(userOverrideAmount)) {
			return;
		}
		onUpsertOverride({
			userID: targetUserID,
			req: { spend_limit_micros: dollarsToMicros(userOverrideAmount) },
			onSuccess: () => {
				setEditingUserOverride(null);
				setSelectedUserOverride(null);
				setUserOverrideAmount("");
				setShowUserForm(false);
			},
		});
	};

	const handleAddGroupOverride = () => {
		const targetGroupID = editingGroupOverride?.group_id ?? selectedGroup?.id;

		if (!targetGroupID || !isPositiveFiniteDollarAmount(groupAmount)) {
			return;
		}
		onUpsertGroupOverride({
			groupID: targetGroupID,
			req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			onSuccess: () => {
				setEditingGroupOverride(null);
				setSelectedGroup(null);
				setGroupAmount("");
				setShowGroupForm(false);
			},
		});
	};

	const handleDeleteGroupOverride = (groupID: string) => {
		onDeleteGroupOverride(groupID);
	};

	const handleDeleteOverride = (userID: string) => {
		onDeleteOverride(userID);
	};

	// ── Loading / error states for config ──
	if (isLoadingConfig) {
		return (
			<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div className="flex flex-1 items-center justify-center px-6 py-5">
					<Spinner loading className="h-6 w-6" />
				</div>
			</div>
		);
	}

	if (configError) {
		return (
			<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div className="flex flex-1 items-center justify-center px-6 py-5">
					<div className="space-y-4 py-4 text-center">
						<p className="text-sm text-content-secondary">
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
				</div>
			</div>
		);
	}

	const groupOverrides = configData?.group_overrides ?? [];
	const overrides = configData?.overrides ?? [];
	const unpricedModelCount = configData?.unpriced_model_count ?? 0;

	if (selectedUserId) {
		return (
			<SpendDrillInView
				selectedUser={selectedUser}
				isLoading={isSelectedUserLoading}
				isError={isSelectedUserError}
				error={selectedUserError}
				onRetry={onSelectedUserRetry}
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
		<div className="space-y-6">
			<SectionHeader
				label="Spend management"
				description="Configure spend limits and monitor usage across your deployment."
				badge={<AdminBadge />}
			/>

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
					<CollapsibleSection
						title="Default spend limit"
						description="Set a deployment-wide spend cap that applies to all users by default."
					>
						<DefaultLimitSection
							hideHeader
							adminBadge={null}
							enabled={enabled}
							onEnabledChange={(nextEnabled) => {
								handleResetUpdateConfig();
								onEnabledChange(nextEnabled);
							}}
							period={period}
							onPeriodChange={(nextPeriod) => {
								handleResetUpdateConfig();
								onPeriodChange(nextPeriod);
							}}
							amountDollars={amountDollars}
							onAmountDollarsChange={(nextAmountDollars) => {
								handleResetUpdateConfig();
								onAmountDollarsChange(nextAmountDollars);
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
					</CollapsibleSection>
				)}
			</DefaultLimitController>

			{/* Section 2: Group limits */}
			<CollapsibleSection
				title="Group limits"
				description="Override the default limit for specific groups. The lowest group limit applies."
			>
				<GroupLimitsSection
					hideHeader
					groupOverrides={groupOverrides}
					showGroupForm={showGroupForm}
					onShowGroupFormChange={handleShowGroupFormChange}
					selectedGroup={selectedGroup}
					onSelectedGroupChange={setSelectedGroup}
					groupAmount={groupAmount}
					onGroupAmountChange={setGroupAmount}
					availableGroups={availableGroups}
					groupAutocompleteNoOptionsText={groupAutocompleteNoOptionsText}
					groupsLoading={isLoadingGroups}
					editingGroupOverride={editingGroupOverride}
					onEditGroupOverride={handleEditGroupOverride}
					onAddGroupOverride={handleAddGroupOverride}
					onDeleteGroupOverride={handleDeleteGroupOverride}
					upsertPending={isUpsertingGroupOverride}
					upsertError={upsertGroupOverrideError}
					deletePending={isDeletingGroupOverride}
					deleteError={deleteGroupOverrideError}
					groupsError={groupsError}
				/>
			</CollapsibleSection>

			{/* Section 3: Per-user spend */}
			<CollapsibleSection
				title="Per-user spend"
				description="User overrides take highest priority, followed by group limits, then the default."
				action={
					<DateRangePicker
						value={displayDateRange}
						onChange={onDateRangeChange}
					/>
				}
			>
				<UserOverridesSection
					hideHeader
					overrides={overrides}
					showUserForm={showUserForm}
					onShowUserFormChange={handleShowUserFormChange}
					selectedUser={selectedUserOverride}
					onSelectedUserChange={setSelectedUserOverride}
					userOverrideAmount={userOverrideAmount}
					onUserOverrideAmountChange={setUserOverrideAmount}
					selectedUserAlreadyOverridden={
						editingUserOverride ? false : selectedUserAlreadyOverridden
					}
					editingUserOverride={editingUserOverride}
					onEditUserOverride={handleEditUserOverride}
					onAddOverride={handleAddOverride}
					onDeleteOverride={handleDeleteOverride}
					upsertPending={isUpsertingOverride}
					upsertError={upsertOverrideError}
					deletePending={isDeletingOverride}
					deleteError={deleteOverrideError}
				/>
				{/* Search + pagination amount */}
				<div className="flex flex-col gap-3 pt-6 md:flex-row md:items-center md:justify-between">
					<div className="w-full md:max-w-sm">
						<SearchField
							value={searchFilter}
							onChange={(value) => {
								onSearchFilterChange(value);
								onPageChange(1);
							}}
							placeholder="Search by name or username"
							aria-label="Search usage by name or username"
						/>
					</div>
					{usersData && (
						<PaginationAmount
							limit={pageSize}
							totalRecords={usersData.count}
							currentOffsetStart={usersData.count === 0 ? 0 : offset + 1}
							paginationUnitLabel="users"
						/>
					)}
				</div>
				{/* Loading state */}
				{isUsersLoading && (
					<div
						role="status"
						aria-label="Loading usage"
						className="flex min-h-[240px] items-center justify-center"
					>
						<Spinner size="lg" loading className="text-content-secondary" />
					</div>
				)}
				{/* Error state */}
				{usersError != null && (
					<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
						<p className="m-0 text-sm text-content-secondary">
							{getErrorMessage(usersError, "Failed to load usage data.")}
						</p>
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={onUsersRetry}
						>
							Retry
						</Button>
					</div>
				)}
				{/* User table */}
				{usersData && (
					<div className="relative pt-3">
						{isUsersFetching && !isUsersLoading && (
							<div
								role="status"
								aria-label="Refreshing usage"
								className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
							>
								<Spinner size="lg" loading className="text-content-secondary" />
							</div>
						)}
						{usersData.users.length === 0 ? (
							<p className="py-12 text-center text-content-secondary">
								No usage data for this period.
							</p>
						) : (
							<>
								<div className="overflow-hidden rounded-lg border border-border-default">
									<Table aria-label="Per-user spend">
										<TableHeader>
											<TableRow>
												<TableHead>User</TableHead>
												<TableHead className="text-right">Cost</TableHead>
												<TableHead className="text-right">Messages</TableHead>
												<TableHead className="text-right">Chats</TableHead>
												<TableHead className="text-right">Input</TableHead>
												<TableHead className="text-right">Output</TableHead>
												<TableHead className="text-right">Cache Read</TableHead>
												<TableHead className="text-right">
													Cache Write
												</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{usersData.users.map((user) => (
												<UserRow
													key={user.user_id}
													user={user}
													onSelect={onSelectUser}
												/>
											))}
										</TableBody>
									</Table>
								</div>
								<div className="pt-4">
									<PaginationWidgetBase
										totalRecords={usersData.count}
										currentPage={page}
										pageSize={pageSize}
										onPageChange={onPageChange}
										hasPreviousPage={hasPreviousPage}
										hasNextPage={hasNextPage}
									/>
								</div>
							</>
						)}
					</div>
				)}
			</CollapsibleSection>
		</div>
	);
};
