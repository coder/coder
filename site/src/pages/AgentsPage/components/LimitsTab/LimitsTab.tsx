import { ShieldIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";

import { getErrorMessage } from "#/api/errors";
import type {
	ChatUsageLimitConfigResponse,
	ChatUsageLimitPeriod,
	Group,
	UpsertChatUsageLimitGroupOverrideRequest,
	UpsertChatUsageLimitOverrideRequest,
	User,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
} from "#/utils/currency";
import { DefaultLimitSection } from "./DefaultLimitSection";
import { GroupLimitsSection } from "./GroupLimitsSection";
import { normalizeChatUsageLimitPeriod } from "./limitsFormLogic";
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

const AdminBadge: FC = () => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex cursor-default items-center gap-1 rounded bg-surface-tertiary/60 px-1.5 py-px text-[11px] font-medium text-content-secondary">
					<ShieldIcon className="h-3 w-3" />
					Admin
				</span>
			</TooltipTrigger>
			<TooltipContent side="right">
				Only visible to deployment administrators.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

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

interface LimitsTabProps {
	// Config query data.
	configData: ChatUsageLimitConfigResponse | undefined;
	isLoadingConfig: boolean;
	configError: Error | null;
	refetchConfig: () => void;
	// Groups query data.
	groupsData: Group[] | undefined;
	isLoadingGroups: boolean;
	groupsError: Error | null;
	// Update config mutation.
	onUpdateConfig: (
		req: import("#/api/typesGenerated").ChatUsageLimitConfig,
	) => Promise<unknown>;
	isUpdatingConfig: boolean;
	updateConfigError: Error | null;
	isUpdateConfigSuccess: boolean;
	resetUpdateConfig: () => void;
	// Upsert user override mutation.
	onUpsertOverride: (args: {
		userID: string;
		req: UpsertChatUsageLimitOverrideRequest;
	}) => Promise<unknown>;
	isUpsertingOverride: boolean;
	upsertOverrideError: Error | null;
	// Delete user override mutation.
	onDeleteOverride: (userID: string) => Promise<unknown>;
	isDeletingOverride: boolean;
	deleteOverrideError: Error | null;
	// Upsert group override mutation.
	onUpsertGroupOverride: (args: {
		groupID: string;
		req: UpsertChatUsageLimitGroupOverrideRequest;
	}) => Promise<unknown>;
	isUpsertingGroupOverride: boolean;
	upsertGroupOverrideError: Error | null;
	// Delete group override mutation.
	onDeleteGroupOverride: (groupID: string) => Promise<unknown>;
	isDeletingGroupOverride: boolean;
	deleteGroupOverrideError: Error | null;
}

export const LimitsTab: FC<LimitsTabProps> = ({
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
}) => {
	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
	const [groupAmount, setGroupAmount] = useState("");
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedUser, setSelectedUser] = useState<User | null>(null);
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
	const selectedUserAlreadyOverridden = selectedUser
		? existingUserIds.has(selectedUser.id)
		: false;
	const groupAutocompleteNoOptionsText = isLoadingGroups
		? "Loading groups..."
		: (groupsData?.length ?? 0) === 0
			? "No groups configured"
			: availableGroups.length === 0
				? "All groups already have overrides"
				: "No groups available";

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
		setSelectedUser(null);
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
			await onUpdateConfig({
				spend_limit_micros: spendLimitMicros,
				period,
				updated_at: new Date().toISOString(),
			});
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleAddOverride = async () => {
		const targetUserID = editingUserOverride?.user_id ?? selectedUser?.id;

		if (!targetUserID || !isPositiveFiniteDollarAmount(userOverrideAmount)) {
			return;
		}
		try {
			await onUpsertOverride({
				userID: targetUserID,
				req: { spend_limit_micros: dollarsToMicros(userOverrideAmount) },
			});
			setEditingUserOverride(null);
			setSelectedUser(null);
			setUserOverrideAmount("");
			setShowUserForm(false);
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleAddGroupOverride = async () => {
		const targetGroupID = editingGroupOverride?.group_id ?? selectedGroup?.id;

		if (!targetGroupID || !isPositiveFiniteDollarAmount(groupAmount)) {
			return;
		}
		try {
			await onUpsertGroupOverride({
				groupID: targetGroupID,
				req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			});
			setEditingGroupOverride(null);
			setSelectedGroup(null);
			setGroupAmount("");
			setShowGroupForm(false);
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleDeleteGroupOverride = async (groupID: string) => {
		try {
			await onDeleteGroupOverride(groupID);
		} catch {
			// Keep the current UI state so the inline mutation error is visible.
		}
	};

	const handleDeleteOverride = async (userID: string) => {
		try {
			await onDeleteOverride(userID);
		} catch {
			// Keep the current UI state so the inline mutation error is visible.
		}
	};

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
								<DefaultLimitSection
									adminBadge={<AdminBadge />}
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
								<UserOverridesSection
									overrides={overrides}
									showUserForm={showUserForm}
									onShowUserFormChange={handleShowUserFormChange}
									selectedUser={selectedUser}
									onSelectedUserChange={setSelectedUser}
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
							</div>
						</div>
						<div className="sticky bottom-0 flex shrink-0 flex-col gap-2 border-t border-border bg-surface-primary py-3 sm:flex-row sm:items-center sm:justify-between">
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
								onClick={() => void saveDefault()}
								disabled={isUpdatingConfig || !isAmountValid}
							>
								{isUpdatingConfig ? (
									<Spinner loading className="h-4 w-4" />
								) : null}
								Save default limit
							</Button>
						</div>{" "}
					</>
				)}
			</DefaultLimitController>
		</div>
	);
};
