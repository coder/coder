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
import type * as TypesGen from "api/typesGenerated";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { ShieldIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { formatCostMicros } from "utils/analytics";
import { SectionHeader } from "./SectionHeader";

const microsToDollars = (micros: number): number =>
	Math.round(micros / 10_000) / 100;

const dollarsToMicros = (dollars: string): number =>
	Math.round(Number(dollars) * 1_000_000);

const inputClassName =
	"w-full rounded-lg border border-border bg-surface-primary px-3 py-2 text-[13px] text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";

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
	const [period, setPeriod] = useState<TypesGen.ChatUsageLimitPeriod>("month");
	const [amountDollars, setAmountDollars] = useState("");
	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroupId, setSelectedGroupId] = useState("");
	const [groupAmount, setGroupAmount] = useState("");
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedUser, setSelectedUser] = useState<TypesGen.User | null>(null);
	const [userOverrideAmount, setUserOverrideAmount] = useState("");
	const lastSyncedRef = useRef<string | null>(null);

	useEffect(() => {
		if (!configQuery.data) {
			return;
		}

		const snapshot = JSON.stringify({
			spend_limit_micros: configQuery.data.spend_limit_micros,
			period: configQuery.data.period,
		});
		if (lastSyncedRef.current === snapshot) {
			return;
		}
		lastSyncedRef.current = snapshot;

		const spendLimitMicros = configQuery.data.spend_limit_micros;
		const hasLimit = spendLimitMicros !== null;
		setEnabled(hasLimit);
		setPeriod(configQuery.data.period ?? "month");
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

	const isAmountValid =
		!enabled ||
		(amountDollars.trim() !== "" &&
			!Number.isNaN(Number(amountDollars)) &&
			Number(amountDollars) >= 0);

	const handleSaveDefault = async () => {
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
		if (!selectedUser) {
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
		try {
			await upsertGroupOverrideMutation.mutateAsync({
				groupID: selectedGroupId,
				req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			});
			setSelectedGroupId("");
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
					<SectionHeader
						label="Default Spend Limit"
						description="Set a deployment-wide spend cap that applies to all users by default."
						badge={<AdminBadge />}
					/>

					<div className="space-y-4 rounded-lg border border-border p-4">
						<div className="flex items-center justify-between gap-4">
							<div>
								<p className="m-0 text-sm font-medium text-content-primary">
									Enable spend limit
								</p>
								<p className="m-0 text-xs text-content-secondary">
									When disabled, users have unlimited spending.
								</p>
							</div>
							<Switch checked={enabled} onCheckedChange={setEnabled} />
						</div>

						{enabled && (
							<div className="flex flex-col gap-3 md:flex-row md:items-end">
								<div className="flex-1">
									<label
										className="mb-1 block text-xs font-medium text-content-secondary"
										htmlFor="chat-limit-period"
									>
										Period
									</label>
									<select
										id="chat-limit-period"
										className={inputClassName}
										value={period}
										onChange={(event) =>
											setPeriod(
												event.target.value as TypesGen.ChatUsageLimitPeriod,
											)
										}
									>
										<option value="day">Day</option>
										<option value="week">Week</option>
										<option value="month">Month</option>
									</select>
								</div>
								<div className="flex-1">
									<label
										className="mb-1 block text-xs font-medium text-content-secondary"
										htmlFor="chat-limit-amount"
									>
										Amount ($)
									</label>
									<input
										id="chat-limit-amount"
										type="number"
										step="0.01"
										min="0"
										className={inputClassName}
										value={amountDollars}
										onChange={(event) => setAmountDollars(event.target.value)}
										placeholder="0.00"
									/>
								</div>
							</div>
						)}
					</div>

					{enabled && unpricedModelCount > 0 && (
						<div className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary">
							<TriangleAlertIcon className="h-5 w-5 shrink-0 text-content-warning" />
							<div>
								{unpricedModelCount === 1
									? "1 enabled model does not have pricing configured."
									: `${unpricedModelCount} enabled models do not have pricing configured.`}{" "}
								Usage of unpriced models cannot be tracked against the spend
								limit.
							</div>
						</div>
					)}

					<SectionHeader
						label="Group Limits"
						description="Override the default limit for specific groups."
					/>

					{groupOverrides.length > 0 ? (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Group</TableHead>
									<TableHead>Members</TableHead>
									<TableHead>Spend Limit</TableHead>
									<TableHead className="w-[80px]">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{groupOverrides.map((override) => (
									<TableRow key={override.group_id}>
										<TableCell>
											<AvatarData
												title={
													override.group_display_name || override.group_name
												}
												subtitle={override.group_name}
												src={override.group_avatar_url}
												imgFallbackText={override.group_name}
											/>
										</TableCell>
										<TableCell>{override.member_count}</TableCell>
										<TableCell>
											{override.spend_limit_micros !== null
												? formatCostMicros(override.spend_limit_micros)
												: "Unlimited"}
										</TableCell>
										<TableCell>
											<Button
												variant="outline"
												size="sm"
												type="button"
												onClick={() =>
													void handleDeleteGroupOverride(override.group_id)
												}
												disabled={deleteGroupOverrideMutation.isPending}
											>
												Delete
											</Button>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					) : (
						<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
							No group overrides configured.
						</div>
					)}

					{deleteGroupOverrideMutation.isError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(
								deleteGroupOverrideMutation.error,
								"Failed to delete group override.",
							)}
						</p>
					)}

					{!showGroupForm ? (
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => setShowGroupForm(true)}
							disabled={groupsQuery.isLoading || availableGroups.length === 0}
						>
							Add Group
						</Button>
					) : (
						<div className="flex flex-col gap-3 md:flex-row md:items-end">
							<div className="flex-1">
								<label
									className="mb-1 block text-xs font-medium text-content-secondary"
									htmlFor="group-override-select"
								>
									Group
								</label>
								<select
									id="group-override-select"
									className={inputClassName}
									value={selectedGroupId}
									onChange={(event) => setSelectedGroupId(event.target.value)}
								>
									<option value="">Select a group…</option>
									{availableGroups.map((group) => (
										<option key={group.id} value={group.id}>
											{group.display_name || group.name}
										</option>
									))}
								</select>
							</div>
							<div className="flex-1">
								<label
									className="mb-1 block text-xs font-medium text-content-secondary"
									htmlFor="group-override-amount"
								>
									Spend Limit ($)
								</label>
								<input
									id="group-override-amount"
									type="number"
									step="0.01"
									min="0"
									className={inputClassName}
									value={groupAmount}
									onChange={(event) => setGroupAmount(event.target.value)}
									placeholder="0.00"
								/>
							</div>
							<div className="flex gap-2">
								<Button
									size="sm"
									type="button"
									onClick={() => void handleAddGroupOverride()}
									disabled={
										upsertGroupOverrideMutation.isPending ||
										selectedGroupId === "" ||
										groupAmount.trim() === "" ||
										Number.isNaN(Number(groupAmount)) ||
										Number(groupAmount) < 0
									}
								>
									{upsertGroupOverrideMutation.isPending ? (
										<Spinner loading className="h-4 w-4" />
									) : null}
									Add
								</Button>
								<Button
									variant="outline"
									size="sm"
									type="button"
									onClick={() => {
										setShowGroupForm(false);
										setSelectedGroupId("");
										setGroupAmount("");
									}}
								>
									Cancel
								</Button>
							</div>
						</div>
					)}
					{upsertGroupOverrideMutation.isError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(
								upsertGroupOverrideMutation.error,
								"Failed to save group override.",
							)}
						</p>
					)}
					{groupsQuery.isError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(groupsQuery.error, "Failed to load groups.")}
						</p>
					)}

					<SectionHeader
						label="Per-User Overrides"
						description="Override the deployment default spend limit for specific users."
					/>

					{overrides.length > 0 ? (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>User</TableHead>
									<TableHead>Spend Limit</TableHead>
									<TableHead className="w-[80px]">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{overrides.map((override) => (
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
										<TableCell>
											<Button
												variant="outline"
												size="sm"
												type="button"
												onClick={() =>
													void handleDeleteOverride(override.user_id)
												}
												disabled={deleteOverrideMutation.isPending}
											>
												Delete
											</Button>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					) : (
						<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
							No overrides configured.
						</div>
					)}

					{deleteOverrideMutation.isError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(
								deleteOverrideMutation.error,
								"Failed to delete override.",
							)}
						</p>
					)}

					{!showUserForm ? (
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => setShowUserForm(true)}
						>
							Add User
						</Button>
					) : (
						<div className="flex flex-col gap-3 md:flex-row md:items-end">
							<div className="flex-1">
								<UserAutocomplete
									value={selectedUser}
									onChange={setSelectedUser}
									label="User"
								/>
							</div>
							<div className="flex-1">
								<label
									className="mb-1 block text-xs font-medium text-content-secondary"
									htmlFor="user-override-amount"
								>
									Spend Limit ($)
								</label>
								<input
									id="user-override-amount"
									type="number"
									step="0.01"
									min="0"
									className={inputClassName}
									value={userOverrideAmount}
									onChange={(event) =>
										setUserOverrideAmount(event.target.value)
									}
									placeholder="0.00"
								/>
							</div>
							<div className="flex gap-2">
								<Button
									size="sm"
									type="button"
									onClick={() => void handleAddOverride()}
									disabled={
										upsertOverrideMutation.isPending ||
										!selectedUser ||
										selectedUserAlreadyOverridden ||
										userOverrideAmount.trim() === "" ||
										Number.isNaN(Number(userOverrideAmount)) ||
										Number(userOverrideAmount) < 0
									}
								>
									{upsertOverrideMutation.isPending ? (
										<Spinner loading className="h-4 w-4" />
									) : null}
									Add
								</Button>
								<Button
									variant="outline"
									size="sm"
									type="button"
									onClick={() => {
										setShowUserForm(false);
										setSelectedUser(null);
										setUserOverrideAmount("");
									}}
								>
									Cancel
								</Button>
							</div>
						</div>
					)}
					{selectedUserAlreadyOverridden && (
						<p className="text-xs text-content-warning">
							This user already has an override.
						</p>
					)}
					{upsertOverrideMutation.isError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(
								upsertOverrideMutation.error,
								"Failed to save the override.",
							)}
						</p>
					)}
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
