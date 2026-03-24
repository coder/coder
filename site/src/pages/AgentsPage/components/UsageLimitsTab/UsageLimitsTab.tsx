import { getErrorMessage } from "api/errors";
import {
	chatUsageLimitConfig,
	deleteChatUsageLimitGroupOverride,
	deleteChatUsageLimitOverride,
	updateChatUsageLimitConfig,
	upsertChatUsageLimitGroupOverride,
	upsertChatUsageLimitOverride,
} from "api/queries/chats";
import { groups } from "api/queries/groups";
import type { ChatUsageLimitPeriod, Group, User } from "api/typesGenerated";
import type { Dayjs } from "dayjs";
import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
} from "utils/currency";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { AdminBadge } from "../AdminBadge";
import type { DateRangeValue } from "../DateRangePicker/DateRangePicker";
import { SectionHeader } from "../SectionHeader";
import { DefaultLimitSection } from "./DefaultLimitSection";
import { GroupLimitsSection } from "./GroupLimitsSection";
import { normalizeChatUsageLimitPeriod } from "./limitsFormLogic";
import {
	formatUsageDateRange,
	getDefaultUsageDateRange,
	UsageUsersSection,
	usageEndDateSearchParam,
	usageStartDateSearchParam,
} from "./UsageUsersSection";
import { UserOverridesSection } from "./UserOverridesSection";

interface DefaultLimitFormValues {
	enabled: boolean;
	period: ChatUsageLimitPeriod;
	amountDollars: string;
}

interface DefaultLimitControllerProps {
	initialValues: DefaultLimitFormValues;
	onSave: (values: DefaultLimitFormValues) => Promise<void>;
	children: (props: {
		enabled: boolean;
		onEnabledChange: (enabled: boolean) => void;
		period: ChatUsageLimitPeriod;
		onPeriodChange: (period: ChatUsageLimitPeriod) => void;
		amountDollars: string;
		onAmountDollarsChange: (amount: string) => void;
		isAmountValid: boolean;
		saveDefault: () => Promise<void>;
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

	const handleSave = async () => {
		if (enabled && !isPositiveFiniteDollarAmount(amountDollars)) {
			return;
		}
		await onSave({ enabled, period, amountDollars });
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

// ── Main component ─────────────────────────────────────────────────

interface UsageLimitsTabProps {
	now?: Dayjs;
}

export const UsageLimitsTab: FC<UsageLimitsTabProps> = ({ now }) => {
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();

	// ── Limit config queries & mutations ───────────────────────────

	const configQuery = useQuery(chatUsageLimitConfig());
	const updateConfigMutation = useMutation(
		updateChatUsageLimitConfig(queryClient),
	);
	const upsertOverrideMutation = useMutation(
		upsertChatUsageLimitOverride(queryClient),
	);
	const deleteOverrideMutation = useMutation(
		deleteChatUsageLimitOverride(queryClient),
	);
	const groupsQuery = useQuery(groups());
	const upsertGroupOverrideMutation = useMutation(
		upsertChatUsageLimitGroupOverride(queryClient),
	);
	const deleteGroupOverrideMutation = useMutation(
		deleteChatUsageLimitGroupOverride(queryClient),
	);

	// ── Limit form state ───────────────────────────────────────────

	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
	const [groupAmount, setGroupAmount] = useState("");
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedOverrideUser, setSelectedOverrideUser] = useState<User | null>(
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

	// ── Usage date range state ─────────────────────────────────────

	const startDateParam =
		searchParams.get(usageStartDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(usageEndDateSearchParam)?.trim() ?? "";
	const [defaultDateRange] = useState(() => getDefaultUsageDateRange(now));

	let dateRange = defaultDateRange;
	let hasExplicitDateRange = false;

	if (startDateParam && endDateParam) {
		const parsedStartDate = new Date(startDateParam);
		const parsedEndDate = new Date(endDateParam);

		if (
			!Number.isNaN(parsedStartDate.getTime()) &&
			!Number.isNaN(parsedEndDate.getTime()) &&
			parsedStartDate.getTime() <= parsedEndDate.getTime()
		) {
			dateRange = {
				startDate: parsedStartDate,
				endDate: parsedEndDate,
			};
			hasExplicitDateRange = true;
		}
	}

	const dateRangeParams = {
		start_date: dateRange.startDate.toISOString(),
		end_date: dateRange.endDate.toISOString(),
	};
	const { endDate } = dateRange;
	const isExclusiveMidnightEnd =
		hasExplicitDateRange &&
		endDate.getHours() === 0 &&
		endDate.getMinutes() === 0 &&
		endDate.getSeconds() === 0 &&
		endDate.getMilliseconds() === 0;
	const displayDateRange = isExclusiveMidnightEnd
		? {
				startDate: dateRange.startDate,
				endDate: new Date(endDate.getTime() - 1),
			}
		: dateRange;
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive: hasExplicitDateRange,
	});

	const onDateRangeChange = (value: DateRangeValue) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set(usageStartDateSearchParam, value.startDate.toISOString());
			next.set(usageEndDateSearchParam, value.endDate.toISOString());
			return next;
		});
	};

	// ── Derived limit config values ────────────────────────────────

	const configData = configQuery.data;

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
	const availableGroups = (groupsQuery.data ?? []).filter(
		(g) => !existingGroupIds.has(g.id),
	);
	const selectedUserAlreadyOverridden = selectedOverrideUser
		? existingUserIds.has(selectedOverrideUser.id)
		: false;
	const groupAutocompleteNoOptionsText = groupsQuery.isLoading
		? "Loading groups..."
		: (groupsQuery.data?.length ?? 0) === 0
			? "No groups configured"
			: availableGroups.length === 0
				? "All groups already have overrides"
				: "No groups available";

	// ── Limit mutation handlers ────────────────────────────────────

	const resetUpdateConfigMutation = () => {
		if (!updateConfigMutation.isPending) {
			updateConfigMutation.reset();
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
		setSelectedOverrideUser(null);
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

	const handleSaveDefault = async ({
		enabled,
		period,
		amountDollars,
	}: DefaultLimitFormValues) => {
		const spendLimitMicros = enabled ? dollarsToMicros(amountDollars) : null;
		try {
			await updateConfigMutation.mutateAsync({
				spend_limit_micros: spendLimitMicros,
				period,
				updated_at: new Date().toISOString(),
			});
		} catch {
			// Keep form state so the inline mutation error is visible.
		}
	};

	const handleAddOverride = async () => {
		const targetUserID =
			editingUserOverride?.user_id ?? selectedOverrideUser?.id;

		if (!targetUserID || !isPositiveFiniteDollarAmount(userOverrideAmount)) {
			return;
		}
		try {
			await upsertOverrideMutation.mutateAsync({
				userID: targetUserID,
				req: {
					spend_limit_micros: dollarsToMicros(userOverrideAmount),
				},
			});
			setEditingUserOverride(null);
			setSelectedOverrideUser(null);
			setUserOverrideAmount("");
			setShowUserForm(false);
		} catch {
			// Keep form state so the inline mutation error is visible.
		}
	};

	const handleAddGroupOverride = async () => {
		const targetGroupID = editingGroupOverride?.group_id ?? selectedGroup?.id;

		if (!targetGroupID || !isPositiveFiniteDollarAmount(groupAmount)) {
			return;
		}
		try {
			await upsertGroupOverrideMutation.mutateAsync({
				groupID: targetGroupID,
				req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			});
			setEditingGroupOverride(null);
			setSelectedGroup(null);
			setGroupAmount("");
			setShowGroupForm(false);
		} catch {
			// Keep form state so the inline mutation error is visible.
		}
	};

	const handleDeleteGroupOverride = async (groupID: string) => {
		try {
			await deleteGroupOverrideMutation.mutateAsync(groupID);
		} catch {
			// Keep UI state so the inline mutation error is visible.
		}
	};

	const handleDeleteOverride = async (userID: string) => {
		try {
			await deleteOverrideMutation.mutateAsync(userID);
		} catch {
			// Keep UI state so the inline mutation error is visible.
		}
	};

	// ── Loading state ──────────────────────────────────────────────

	if (configQuery.isLoading) {
		return (
			<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div className="flex flex-1 items-center justify-center px-6 py-5">
					<Spinner loading className="h-6 w-6" />
				</div>
			</div>
		);
	}

	// ── Error state ────────────────────────────────────────────────

	if (configQuery.isError) {
		return (
			<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div className="flex flex-1 items-center justify-center px-6 py-5">
					<div className="space-y-4 py-4 text-center">
						<p className="text-sm text-content-secondary">
							{getErrorMessage(
								configQuery.error,
								"Failed to load spend limit settings.",
							)}
						</p>
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => void configQuery.refetch()}
						>
							Retry
						</Button>
					</div>
				</div>
			</div>
		);
	}

	// ── Shared props for UsageUsersSection ──────────────────────────

	const usersSectionProps = {
		dateRangeParams,
		dateRangeLabel,
		displayDateRange,
		onDateRangeChange,
		configData,
	};
	// ── Drill-in: selected user detail view ────────────────────────

	const selectedUserId = searchParams.get("user");
	if (selectedUserId) {
		return <UsageUsersSection {...usersSectionProps} />;
	}

	// ── Normal view: combined usage + limits ───────────────────────

	const groupOverrides = configData?.group_overrides ?? [];
	const overrides = configData?.overrides ?? [];
	const unpricedModelCount = configData?.unpriced_model_count ?? 0;

	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
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
					<>
						<div className="flex-1 overflow-y-auto pb-24 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
							<div className="space-y-10">
								{/* ── Section: header with date picker ── */}
								<SectionHeader
									label="Usage & Limits"
									description="Manage deployment spend limits and review chat usage."
									badge={<AdminBadge className="ml-auto" />}
								/>
								{/* ── Section: default spend limit ── */}
								<DefaultLimitSection
									enabled={enabled}
									onEnabledChange={(nextEnabled) => {
										resetUpdateConfigMutation();
										onEnabledChange(nextEnabled);
									}}
									period={period}
									onPeriodChange={(nextPeriod) => {
										resetUpdateConfigMutation();
										onPeriodChange(nextPeriod);
									}}
									amountDollars={amountDollars}
									onAmountDollarsChange={(nextAmountDollars) => {
										resetUpdateConfigMutation();
										onAmountDollarsChange(nextAmountDollars);
									}}
									unpricedModelCount={unpricedModelCount}
								/>

								{/* ── Sticky save bar for default limit ── */}
								<div className="sticky bottom-0 z-10 flex shrink-0 flex-col gap-2 border-t border-border bg-surface-primary py-3 sm:flex-row sm:items-center sm:justify-between">
									<div className="min-h-4 text-xs">
										{updateConfigMutation.isError && (
											<p className="m-0 text-content-destructive">
												{getErrorMessage(
													updateConfigMutation.error,
													"Failed to save the default spend limit.",
												)}
											</p>
										)}
										{updateConfigMutation.isSuccess && (
											<p className="m-0 text-content-success">Saved!</p>
										)}
									</div>
									<Button
										size="sm"
										type="button"
										onClick={() => void saveDefault()}
										disabled={updateConfigMutation.isPending || !isAmountValid}
									>
										{updateConfigMutation.isPending ? (
											<Spinner loading className="h-4 w-4" />
										) : null}
										Save default limit
									</Button>
								</div>

								{/* ── Separator ── */}
								<hr className="border-0 border-t border-solid border-border" />

								{/* ── Section: Group limits ── */}
								<GroupLimitsSection
									groupOverrides={groupOverrides}
									showGroupForm={showGroupForm}
									onShowGroupFormChange={handleShowGroupFormChange}
									selectedGroup={selectedGroup}
									onSelectedGroupChange={setSelectedGroup}
									groupAmount={groupAmount}
									onGroupAmountChange={setGroupAmount}
									availableGroups={availableGroups}
									groupAutocompleteNoOptionsText={
										groupAutocompleteNoOptionsText
									}
									groupsLoading={groupsQuery.isLoading}
									editingGroupOverride={editingGroupOverride}
									onEditGroupOverride={handleEditGroupOverride}
									onAddGroupOverride={handleAddGroupOverride}
									onDeleteGroupOverride={handleDeleteGroupOverride}
									upsertPending={upsertGroupOverrideMutation.isPending}
									upsertError={
										upsertGroupOverrideMutation.isError
											? upsertGroupOverrideMutation.error
											: null
									}
									deletePending={deleteGroupOverrideMutation.isPending}
									deleteError={
										deleteGroupOverrideMutation.isError
											? deleteGroupOverrideMutation.error
											: null
									}
									groupsError={groupsQuery.isError ? groupsQuery.error : null}
								/>

								{/* ── Section: Per-user overrides ── */}
								<UserOverridesSection
									overrides={overrides}
									showUserForm={showUserForm}
									onShowUserFormChange={handleShowUserFormChange}
									selectedUser={selectedOverrideUser}
									onSelectedUserChange={setSelectedOverrideUser}
									userOverrideAmount={userOverrideAmount}
									onUserOverrideAmountChange={setUserOverrideAmount}
									selectedUserAlreadyOverridden={
										editingUserOverride ? false : selectedUserAlreadyOverridden
									}
									editingUserOverride={editingUserOverride}
									onEditUserOverride={handleEditUserOverride}
									onAddOverride={handleAddOverride}
									onDeleteOverride={handleDeleteOverride}
									upsertPending={upsertOverrideMutation.isPending}
									upsertError={
										upsertOverrideMutation.isError
											? upsertOverrideMutation.error
											: null
									}
									deletePending={deleteOverrideMutation.isPending}
									deleteError={
										deleteOverrideMutation.isError
											? deleteOverrideMutation.error
											: null
									}
								/>

								{/* ── Separator ── */}
								<hr className="border-0 border-t border-solid border-border" />

								{/* ── Section: Users usage table ── */}
								<UsageUsersSection {...usersSectionProps} />
							</div>{" "}
						</div>
					</>
				)}
			</DefaultLimitController>
		</div>
	);
};
