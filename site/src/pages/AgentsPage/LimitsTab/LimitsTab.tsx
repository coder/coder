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
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ShieldIcon } from "lucide-react";
import { type FC, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { DefaultLimitSection } from "./DefaultLimitSection";
import { GroupLimitsSection } from "./GroupLimitsSection";
import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
	normalizeChatUsageLimitPeriod,
} from "./limitsFormLogic";
import { UserOverridesSection } from "./UserOverridesSection";

const sectionPanelClassName = "space-y-4 rounded-lg border border-border p-4";

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

export const LimitsTab: FC = () => {
	const queryClient = useQueryClient();
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

	const [enabled, setEnabled] = useState(false);
	const [period, setPeriod] = useState<ChatUsageLimitPeriod>("month");
	const [amountDollars, setAmountDollars] = useState("");
	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
	const [groupAmount, setGroupAmount] = useState("");
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedUser, setSelectedUser] = useState<User | null>(null);
	const [userOverrideAmount, setUserOverrideAmount] = useState("");
	const lastSyncedRef = useRef<string | null>(null);

	useEffect(() => {
		if (!configQuery.data) {
			return;
		}

		const normalizedPeriod = normalizeChatUsageLimitPeriod(
			configQuery.data.period,
		);
		const snapshot = JSON.stringify({
			spend_limit_micros: configQuery.data.spend_limit_micros,
			period: normalizedPeriod,
		});
		if (lastSyncedRef.current === snapshot) {
			return;
		}
		lastSyncedRef.current = snapshot;

		const spendLimitMicros = configQuery.data.spend_limit_micros;
		const hasLimit = spendLimitMicros !== null;
		setEnabled(hasLimit);
		setPeriod(normalizedPeriod);
		setAmountDollars(
			hasLimit ? microsToDollars(spendLimitMicros).toString() : "",
		);
	}, [configQuery.data]);

	const existingGroupIds = useMemo(
		() =>
			new Set((configQuery.data?.group_overrides ?? []).map((g) => g.group_id)),
		[configQuery.data?.group_overrides],
	);
	const existingUserIds = useMemo(
		() => new Set((configQuery.data?.overrides ?? []).map((o) => o.user_id)),
		[configQuery.data?.overrides],
	);
	const availableGroups = useMemo(
		() => (groupsQuery.data ?? []).filter((g) => !existingGroupIds.has(g.id)),
		[groupsQuery.data, existingGroupIds],
	);
	const selectedUserAlreadyOverridden = selectedUser
		? existingUserIds.has(selectedUser.id)
		: false;
	const groupAutocompleteNoOptionsText = groupsQuery.isLoading
		? "Loading groups..."
		: availableGroups.length === 0
			? "All groups already have overrides"
			: "No groups available";

	const isAmountValid = !enabled || isPositiveFiniteDollarAmount(amountDollars);

	const handleSaveDefault = async () => {
		if (enabled && !isPositiveFiniteDollarAmount(amountDollars)) {
			return;
		}
		const spendLimitMicros = enabled ? dollarsToMicros(amountDollars) : null;
		try {
			await updateConfigMutation.mutateAsync({
				spend_limit_micros: spendLimitMicros,
				period,
				updated_at: new Date().toISOString(),
			});
			lastSyncedRef.current = JSON.stringify({
				spend_limit_micros: spendLimitMicros,
				period,
			});
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleAddOverride = async () => {
		if (!selectedUser || !isPositiveFiniteDollarAmount(userOverrideAmount)) {
			return;
		}
		try {
			await upsertOverrideMutation.mutateAsync({
				userID: selectedUser.id,
				req: { spend_limit_micros: dollarsToMicros(userOverrideAmount) },
			});
			setSelectedUser(null);
			setUserOverrideAmount("");
			setShowUserForm(false);
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleAddGroupOverride = async () => {
		if (!selectedGroup || !isPositiveFiniteDollarAmount(groupAmount)) {
			return;
		}
		try {
			await upsertGroupOverrideMutation.mutateAsync({
				groupID: selectedGroup.id,
				req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			});
			setSelectedGroup(null);
			setGroupAmount("");
			setShowGroupForm(false);
		} catch {
			// Keep the current form state so the inline mutation error is visible.
		}
	};

	const handleDeleteGroupOverride = async (groupID: string) => {
		try {
			await deleteGroupOverrideMutation.mutateAsync(groupID);
		} catch {
			// Keep the current UI state so the inline mutation error is visible.
		}
	};

	const handleDeleteOverride = async (userID: string) => {
		try {
			await deleteOverrideMutation.mutateAsync(userID);
		} catch {
			// Keep the current UI state so the inline mutation error is visible.
		}
	};

	if (configQuery.isLoading) {
		return (
			<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div className="flex flex-1 items-center justify-center px-6 py-5">
					<Spinner loading className="h-6 w-6" />
				</div>
			</div>
		);
	}

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

	const groupOverrides = configQuery.data?.group_overrides ?? [];
	const overrides = configQuery.data?.overrides ?? [];
	const unpricedModelCount = configQuery.data?.unpriced_model_count ?? 0;

	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
			<div className="flex-1 overflow-y-auto px-6 py-5 pb-24 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
				<div className="space-y-6">
					<DefaultLimitSection
						adminBadge={<AdminBadge />}
						enabled={enabled}
						onEnabledChange={setEnabled}
						period={period}
						onPeriodChange={setPeriod}
						amountDollars={amountDollars}
						onAmountDollarsChange={setAmountDollars}
						unpricedModelCount={unpricedModelCount}
						panelClassName={sectionPanelClassName}
					/>

					<GroupLimitsSection
						groupOverrides={groupOverrides}
						panelClassName={sectionPanelClassName}
						showGroupForm={showGroupForm}
						onShowGroupFormChange={setShowGroupForm}
						selectedGroup={selectedGroup}
						onSelectedGroupChange={setSelectedGroup}
						groupAmount={groupAmount}
						onGroupAmountChange={setGroupAmount}
						availableGroups={availableGroups}
						groupAutocompleteNoOptionsText={groupAutocompleteNoOptionsText}
						groupsLoading={groupsQuery.isLoading}
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

					<UserOverridesSection
						overrides={overrides}
						panelClassName={sectionPanelClassName}
						showUserForm={showUserForm}
						onShowUserFormChange={setShowUserForm}
						selectedUser={selectedUser}
						onSelectedUserChange={setSelectedUser}
						userOverrideAmount={userOverrideAmount}
						onUserOverrideAmountChange={setUserOverrideAmount}
						selectedUserAlreadyOverridden={selectedUserAlreadyOverridden}
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
				</div>
			</div>

			<div className="sticky bottom-0 flex shrink-0 flex-col gap-2 border-t border-border bg-surface-primary px-6 py-3 sm:flex-row sm:items-center sm:justify-between">
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
					onClick={() => void handleSaveDefault()}
					disabled={updateConfigMutation.isPending || !isAmountValid}
				>
					{updateConfigMutation.isPending ? (
						<Spinner loading className="h-4 w-4" />
					) : null}
					Save default limit
				</Button>
			</div>
		</div>
	);
};
