import {
	type FC,
	type ReactNode,
	type SyntheticEvent,
	useId,
	useMemo,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail } from "#/api/errors";
import { groupAIBudget, groupsForUser } from "#/api/queries/groups";
import {
	deleteUserAIBudgetOverride,
	saveUserAIBudgetOverride,
	userAIBudgetOverride,
} from "#/api/queries/users";
import type {
	Group,
	GroupAIBudget,
	ReducedUser,
	UpsertUserAIBudgetOverrideRequest,
	UserAIBudgetOverride,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { userIdentity } from "#/components/Avatar/userIdentity";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { Separator } from "#/components/Separator/Separator";
import { Spinner } from "#/components/Spinner/Spinner";
import { aiBudgetRangeError, maxAIBudgetDollars } from "#/modules/groups";
import { cn } from "#/utils/cn";
import {
	dollarsToMicros,
	formatBudgetUSD,
	microsToDollars,
} from "#/utils/currency";

interface UserAIBudgetOverrideDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	user: ReducedUser;
	currentGroup: Group;
	effectiveGroupId?: string | null;
	// When false, the budget is shown without the controls to change it.
	canUpdate: boolean;
}

export const UserAIBudgetOverrideDialog: FC<
	UserAIBudgetOverrideDialogProps
> = ({
	open,
	onOpenChange,
	user,
	currentGroup,
	effectiveGroupId,
	canUpdate,
}) => {
	const queryClient = useQueryClient();
	const budgetOverrideQuery = useQuery({
		...userAIBudgetOverride(user.id),
		enabled: open,
	});
	const userGroupsQuery = useQuery({
		...groupsForUser(user.id, currentGroup.organization_id),
		enabled: open,
	});
	const groupBudgetQuery = useQuery({
		...groupAIBudget(currentGroup.id),
		enabled: open,
	});
	const saveMutation = useMutation(
		saveUserAIBudgetOverride(queryClient, user.id),
	);
	const deleteMutation = useMutation(
		deleteUserAIBudgetOverride(queryClient, user.id),
	);

	const loadError =
		budgetOverrideQuery.error ??
		userGroupsQuery.error ??
		groupBudgetQuery.error;
	const isLoading =
		budgetOverrideQuery.isLoading ||
		userGroupsQuery.isLoading ||
		groupBudgetQuery.isLoading;
	const isSubmitting = saveMutation.isPending || deleteMutation.isPending;
	const budget: BudgetProps = {
		user,
		currentGroup,
		override: budgetOverrideQuery.data ?? null,
		groupBudget: groupBudgetQuery.data ?? null,
		userGroups: userGroupsQuery.data ?? [],
	};

	let body: ReactNode;
	if (loadError) {
		body = <ErrorAlert error={loadError} />;
	} else if (isLoading) {
		body = (
			<div className="flex items-center gap-2 text-sm text-content-secondary">
				<Spinner loading />
				Loading AI budget...
			</div>
		);
	} else if (canUpdate) {
		body = (
			<OverrideForm
				{...budget}
				defaultGroupId={
					effectiveGroupId === undefined ? currentGroup.id : effectiveGroupId
				}
				isSubmitting={isSubmitting}
				onSave={saveMutation.mutateAsync}
				onRemove={deleteMutation.mutateAsync}
				onClose={() => onOpenChange(false)}
			/>
		);
	} else {
		body = <ReadOnlyBudget {...budget} />;
	}

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!isSubmitting) {
					onOpenChange(nextOpen);
				}
			}}
		>
			<DialogContent className="max-w-md gap-5 border-border-default bg-surface-primary p-8 text-content-primary">
				<div className="flex items-start justify-between gap-4">
					<DialogTitle className="font-semibold text-content-primary">
						AI Budget
					</DialogTitle>
					<AvatarData {...userIdentity(user)} />
				</div>

				{body}
			</DialogContent>
		</Dialog>
	);
};

interface BudgetProps {
	user: ReducedUser;
	currentGroup: Group;
	override: UserAIBudgetOverride | null;
	groupBudget: GroupAIBudget | null;
	userGroups: readonly Group[];
}

/** The member's effective limit as a sentence, to place inside a paragraph. */
const BudgetSummary: FC<BudgetProps> = ({
	user,
	currentGroup,
	override,
	groupBudget,
	userGroups,
}) => {
	if (!override) {
		return (
			<>
				{user.username}'s monthly limit is{" "}
				<Bold>
					{groupBudget ? formatUSD(groupBudget.spend_limit_micros) : "uncapped"}
				</Bold>
				, charged to <Bold>{groupDisplayName(currentGroup)}</Bold> group.
			</>
		);
	}

	const overrideGroup = findGroup(currentGroup, userGroups, override.group_id);
	return (
		<>
			{user.username}'s <Bold>custom</Bold> monthly limit is{" "}
			<Bold>{formatUSD(override.spend_limit_micros)}</Bold>, charged to{" "}
			{overrideGroup ? (
				<>
					<Bold>{groupDisplayName(overrideGroup)}</Bold> group.
				</>
			) : (
				// The group is unresolvable here, so it can't be named.
				<Bold>their group.</Bold>
			)}
		</>
	);
};

/**
 * The budget without any editing controls. Setting an override requires
 * updating both the user and the group it charges, so group admins can read a
 * member's budget without being able to change it.
 */
const ReadOnlyBudget: FC<BudgetProps> = (props) => (
	<p className="m-0 text-sm text-content-secondary">
		<BudgetSummary {...props} /> To update this limit, contact a Coder
		administrator.
	</p>
);

interface OverrideFormProps extends BudgetProps {
	// Group marked "(default)" in the picker; null marks none.
	defaultGroupId: string | null;
	isSubmitting: boolean;
	onSave: (request: UpsertUserAIBudgetOverrideRequest) => Promise<unknown>;
	onRemove: () => Promise<unknown>;
	onClose: () => void;
}

/** Mounted only after budget data loads, so state seeds from it without a sync effect. */
const OverrideForm: FC<OverrideFormProps> = ({
	user,
	currentGroup,
	defaultGroupId,
	override,
	groupBudget,
	userGroups,
	isSubmitting,
	onSave,
	onRemove,
	onClose,
}) => {
	const budgetId = useId();
	const groupId = useId();
	const overrideId = useId();

	const [overrideEnabled, setOverrideEnabled] = useState(override !== null);
	// Uncapped (no override or group budget) seeds empty, prompting for a value.
	const [budgetDollars, setBudgetDollars] = useState(() => {
		const seedMicros = (override ?? groupBudget)?.spend_limit_micros;
		return seedMicros === undefined ? "" : String(microsToDollars(seedMicros));
	});
	const [budgetTouched, setBudgetTouched] = useState(false);
	const [selectedGroupId, setSelectedGroupId] = useState(
		override?.group_id ?? currentGroup.id,
	);

	// The current group may also be in the user's groups; dedupe by id.
	const groupOptions = useMemo(() => {
		const byId = new Map<string, Group>([[currentGroup.id, currentGroup]]);
		for (const group of userGroups) {
			byId.set(group.id, group);
		}
		return [...byId.values()].sort((left, right) =>
			groupDisplayName(left).localeCompare(groupDisplayName(right)),
		);
	}, [currentGroup, userGroups]);

	const selectedGroup = groupOptions.find((g) => g.id === selectedGroupId);

	// A "0" budget is valid and disables AI. Empty, negative, or above the
	// configurable maximum is not.
	const budgetAmount = Number(budgetDollars);
	const budgetValid =
		budgetDollars.trim() !== "" &&
		budgetAmount >= 0 &&
		budgetAmount <= maxAIBudgetDollars;
	// Hold the error until the field is touched, so it doesn't flag immediately.
	const budgetInvalid = overrideEnabled && budgetTouched && !budgetValid;
	const budgetDisablesAI = budgetValid && budgetAmount === 0;
	const showFooter = overrideEnabled || override !== null;
	const canSubmit =
		!isSubmitting && (overrideEnabled ? budgetValid : override !== null);

	const groupLabel = (group: Group) =>
		group.id === defaultGroupId
			? `${groupDisplayName(group)} (default)`
			: groupDisplayName(group);

	const handleSubmit = async (event: SyntheticEvent) => {
		event.preventDefault();
		if (!canSubmit) {
			return;
		}

		const removing = !overrideEnabled;
		const mutation = removing
			? onRemove()
			: onSave({
					group_id: selectedGroupId,
					spend_limit_micros: dollarsToMicros(budgetDollars),
				});

		toast.promise(mutation, {
			loading: `${removing ? "Removing" : "Updating"} AI budget override for "${user.username}"...`,
			success: `AI budget override for "${user.username}" ${removing ? "removed" : "updated"} successfully.`,
			error: (error) => ({
				message: `Failed to ${removing ? "remove" : "update"} AI budget override for "${user.username}".`,
				description: getErrorDetail(error),
			}),
		});
		try {
			await mutation;
			onClose();
		} catch {
			// The toast surfaces the error details.
		}
	};

	return (
		<form onSubmit={handleSubmit} className="flex flex-col gap-5">
			<p className="m-0 text-sm text-content-secondary">
				<BudgetSummary
					user={user}
					currentGroup={currentGroup}
					override={override}
					groupBudget={groupBudget}
					userGroups={userGroups}
				/>
			</p>

			<Separator />

			<label
				htmlFor={overrideId}
				className="flex cursor-pointer items-start gap-3"
			>
				<Checkbox
					id={overrideId}
					checked={overrideEnabled}
					onCheckedChange={(checked) => setOverrideEnabled(checked === true)}
					className="mt-0 shrink-0"
				/>
				<div className="flex flex-col gap-1">
					<span className="text-sm font-medium text-content-primary">
						Override group budget
					</span>
					<span className="text-sm text-content-secondary">
						Set a personal limit for this member.
					</span>
				</div>
			</label>

			{overrideEnabled && (
				<>
					<div className="flex flex-col gap-2">
						<Label htmlFor={budgetId}>Custom monthly budget</Label>
						<InputGroup
							className={cn(budgetInvalid && "border-border-destructive")}
						>
							<InputGroupInput
								id={budgetId}
								value={budgetDollars}
								onChange={(event) => setBudgetDollars(event.target.value)}
								onBlur={() => setBudgetTouched(true)}
								type="number"
								min="0"
								max={maxAIBudgetDollars}
								step="1"
								aria-invalid={budgetInvalid}
								aria-describedby={
									budgetInvalid ? `${budgetId}-error` : undefined
								}
							/>
							<InputGroupAddon align="inline-end">USD</InputGroupAddon>
						</InputGroup>
						{budgetInvalid && (
							<p
								id={`${budgetId}-error`}
								className="m-0 text-sm text-content-destructive"
							>
								{aiBudgetRangeError}
							</p>
						)}
					</div>

					{budgetDisablesAI && (
						<Alert severity="info">
							A $0 limit disables AI access for this member.
						</Alert>
					)}

					<div className="flex flex-col gap-2">
						<Label htmlFor={groupId}>Budget assigned to</Label>
						<Combobox
							value={selectedGroupId}
							onValueChange={(value) => {
								// Ignore clearing; a group assignment is always required.
								if (value) {
									setSelectedGroupId(value);
								}
							}}
						>
							<ComboboxTrigger asChild>
								<ComboboxButton
									id={groupId}
									selectedOption={
										selectedGroup && {
											label: groupLabel(selectedGroup),
											value: selectedGroup.id,
											startIcon: (
												<Avatar
													src={selectedGroup.avatar_url}
													fallback={groupDisplayName(selectedGroup)}
												/>
											),
										}
									}
									placeholder="Select a group"
								/>
							</ComboboxTrigger>
							<ComboboxContent
								align="start"
								className="w-[var(--radix-popover-trigger-width)]"
							>
								<ComboboxInput placeholder="Search..." />
								<ComboboxList>
									{groupOptions.map((group) => (
										<ComboboxItem
											key={group.id}
											value={group.id}
											keywords={[groupDisplayName(group)]}
										>
											<span className="flex min-w-0 items-center gap-2">
												<Avatar
													src={group.avatar_url}
													fallback={groupDisplayName(group)}
												/>
												<span className="truncate">{groupLabel(group)}</span>
											</span>
										</ComboboxItem>
									))}
								</ComboboxList>
								<ComboboxEmpty>No groups found</ComboboxEmpty>
							</ComboboxContent>
						</Combobox>
					</div>
				</>
			)}

			{showFooter && (
				<DialogFooter className="mt-4 flex-row justify-end gap-3">
					<Button variant="outline" onClick={onClose} disabled={isSubmitting}>
						Cancel
					</Button>
					<Button type="submit" disabled={!canSubmit}>
						<Spinner loading={isSubmitting} />
						Update
					</Button>
				</DialogFooter>
			)}
		</form>
	);
};

const Bold: FC<{ children: ReactNode }> = ({ children }) => (
	<span className="font-medium text-content-primary">{children}</span>
);

const groupDisplayName = (group: Group): string =>
	group.display_name || group.name;

/**
 * Finds a group among the ones this dialog knows about. Groups in another
 * organization aren't fetchable here, so they resolve to undefined.
 */
const findGroup = (
	currentGroup: Group,
	userGroups: readonly Group[],
	groupID: string,
): Group | undefined =>
	[currentGroup, ...userGroups].find((group) => group.id === groupID);

const formatUSD = (micros: number): string => `${formatBudgetUSD(micros)} USD`;
