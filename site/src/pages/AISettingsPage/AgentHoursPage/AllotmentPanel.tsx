import { PencilIcon, PlusIcon, TrashIcon } from "lucide-react";
import { useState } from "react";
import { Link as RouterLink } from "react-router";
import { toast } from "sonner";
import { DetailedError, getErrorDetail, getErrorStatus } from "#/api/errors";
import { AgentHoursAllotmentMaxBps } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { formatUsedAgentHours, usedAgentHours } from "#/utils/agentHours";
import { getSeverity, usageProgressPercentage } from "#/utils/budget";
import { AllotmentDialog, type AllotmentTarget } from "./AllotmentDialog";
import {
	allotmentHours,
	formatAllotmentPercent,
	formatHours,
} from "./allotments";

type UsageEntry = {
	id: string;
	name: string;
	usedMs: number;
	href?: string;
};

type AllotmentEntry = AllotmentTarget & {
	bps: number;
	usedMs?: number;
	href?: string;
};

export type AllotmentUsage = {
	withoutAllotment: readonly UsageEntry[];
	remainderLabel: string;
	remainderHref?: string;
	remainderUsedMs: number;
	notAttributedMs?: number;
};

type AllotmentPanelProps = {
	entity: "organization" | "group";
	poolLabel: string;
	allotments: readonly AllotmentEntry[] | undefined;
	candidates: readonly AllotmentTarget[];
	/** Undefined when the pool size is unknown. */
	poolHours: number | undefined;
	error: unknown;
	/** Undefined while loading or after a failed load. */
	usage: AllotmentUsage | undefined;
	usageError: unknown;
	onSave: (id: string, bps: number) => Promise<unknown>;
	onRemove: (id: string) => Promise<unknown>;
};

type DialogState = { mode: "add" } | { mode: "edit"; entry: AllotmentEntry };

/** A gauge and table of the allotments that divide one pool. */
export const AllotmentPanel: React.FC<AllotmentPanelProps> = ({
	entity,
	poolLabel,
	allotments,
	candidates,
	poolHours,
	error,
	usage,
	usageError,
	onSave,
	onRemove,
}) => {
	const [dialog, setDialog] = useState<DialogState>();
	const [entryToRemove, setEntryToRemove] = useState<AllotmentEntry>();
	const entityLabel = entity === "group" ? "Group" : "Organization";

	if (allotments === undefined || (usage === undefined && usageError == null)) {
		const loadError = error ?? usageError;
		return loadError != null ? (
			<ErrorAlert error={loadError} />
		) : (
			<Loader label={`Loading ${entity} allotments`} />
		);
	}

	const allottedBps = allotments.reduce((sum, entry) => sum + entry.bps, 0);
	const unallottedBps = Math.max(AgentHoursAllotmentMaxBps - allottedBps, 0);
	const allottedHours = allotmentHours(allottedBps, poolHours);
	const unallottedHours = allotmentHours(unallottedBps, poolHours);
	const showUsage = usage !== undefined;
	const addBlockedReason =
		unallottedBps === 0
			? `All of ${poolLabel} are allotted.`
			: candidates.length === 0
				? `Every ${entity} has an allotment.`
				: undefined;

	const handleRemove = (entry: AllotmentEntry) => {
		setEntryToRemove(undefined);
		toast.promise(
			// A 404 means another admin removed the allotment or deleted its
			// target, which leaves the intended end state.
			onRemove(entry.id).then(
				() => `Removed allotment for ${entry.name}.`,
				(removeError) => {
					if (getErrorStatus(removeError) === 404) {
						return `The allotment for ${entry.name} was already removed.`;
					}
					throw removeError;
				},
			),
			{
				loading: `Removing allotment for ${entry.name}...`,
				success: (message) => message,
				error: (removeError) => ({
					message: `Failed to remove allotment for ${entry.name}.`,
					description:
						getErrorStatus(removeError) === 403
							? "You no longer have access to remove this allotment."
							: getErrorDetail(removeError),
				}),
			},
		);
	};

	// A 404 means the target was deleted or is no longer accessible, so the
	// save can never succeed. Resolving closes the dialog.
	const handleSave = (target: AllotmentTarget, bps: number) =>
		onSave(target.id, bps).catch((saveError: unknown) => {
			const status = getErrorStatus(saveError);
			if (status === 403) {
				throw new DetailedError(
					"You no longer have access to change this allotment.",
				);
			}
			if (status !== 404) {
				throw saveError;
			}
			toast.error(`${target.name} is no longer available.`);
		});

	return (
		<div className="flex flex-col gap-4">
			{error != null && <ErrorAlert error={error} />}
			{usageError != null && <ErrorAlert error={usageError} />}
			<div className="flex flex-col gap-2">
				<UsageBar
					percent={allottedBps / 100}
					ariaLabel={`Allotted share of ${poolLabel}`}
					className="h-2"
				/>
				<p className="m-0 text-sm text-content-secondary">
					<span className="font-medium text-content-primary">
						{formatAllotmentPercent(allottedBps)} allotted
						{allottedHours !== undefined && ` (${formatHours(allottedHours)})`}
					</span>
					, {formatAllotmentPercent(unallottedBps)} unallotted
					{unallottedHours !== undefined &&
						` (${formatHours(unallottedHours)})`}
					{poolHours !== undefined && ` of ${formatHours(poolHours)}`}
				</p>
			</div>

			<Table aria-label={`${entityLabel} allotments`}>
				<TableHeader>
					<TableRow>
						<TableHead>{entityLabel}</TableHead>
						<TableHead>Allotment</TableHead>
						{showUsage && <TableHead>Used</TableHead>}
						<TableHead>
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{allotments.length === 0 && !showUsage ? (
						<TableEmpty
							message={`No ${entity} allotments`}
							description={
								entity === "group"
									? `Every group draws from ${poolLabel}.`
									: "Every organization draws from the shared pool."
							}
							isCompact
						/>
					) : (
						allotments.map((entry) => (
							<AllotmentRow
								key={entry.id}
								entry={entry}
								poolHours={poolHours}
								showUsage={showUsage}
								onEdit={() => setDialog({ mode: "edit", entry })}
								onRemove={() => setEntryToRemove(entry)}
							/>
						))
					)}
					{usage && (
						<UsageRows
							usage={usage}
							unallottedBps={unallottedBps}
							unallottedHours={unallottedHours}
						/>
					)}
				</TableBody>
			</Table>

			<div className="flex items-center gap-3">
				<Button
					variant="outline"
					disabled={addBlockedReason !== undefined}
					onClick={() => setDialog({ mode: "add" })}
				>
					<PlusIcon />
					Add allotment
				</Button>
				{addBlockedReason && (
					<span className="text-sm text-content-secondary">
						{addBlockedReason}
					</span>
				)}
			</div>

			{dialog && (
				<AllotmentDialog
					onClose={() => setDialog(undefined)}
					entity={entity}
					target={dialog.mode === "edit" ? dialog.entry : undefined}
					candidates={candidates}
					// The edited entry may have changed or disappeared since the
					// dialog opened.
					availableBps={
						unallottedBps +
						(dialog.mode === "edit"
							? (allotments.find((entry) => entry.id === dialog.entry.id)
									?.bps ?? 0)
							: 0)
					}
					poolHours={poolHours}
					onSubmit={handleSave}
				/>
			)}

			<ConfirmDialog
				type="delete"
				title="Remove allotment"
				confirmText="Remove"
				description={
					<>
						Remove the allotment for <strong>{entryToRemove?.name}</strong>? Its
						share returns to the unallotted pool.
					</>
				}
				open={entryToRemove !== undefined}
				onClose={() => setEntryToRemove(undefined)}
				onConfirm={() => {
					if (entryToRemove) {
						handleRemove(entryToRemove);
					}
				}}
			/>
		</div>
	);
};

type AllotmentRowProps = {
	entry: AllotmentEntry;
	poolHours: number | undefined;
	showUsage: boolean;
	onEdit: () => void;
	onRemove: () => void;
};

const AllotmentRow: React.FC<AllotmentRowProps> = ({
	entry,
	poolHours,
	showUsage,
	onEdit,
	onRemove,
}) => {
	const hours = allotmentHours(entry.bps, poolHours);
	return (
		<TableRow>
			<TargetNameCell name={entry.name} href={entry.href} />
			<AllotmentCell bps={entry.bps} hours={hours} />
			{showUsage && (
				<UsedCell
					usedMs={entry.usedMs ?? 0}
					rowName={entry.name}
					allottedHours={hours}
				/>
			)}
			<TableCell>
				<div className="flex flex-wrap justify-end gap-2">
					<Button
						variant="outline"
						size="icon"
						aria-label={`Edit allotment for ${entry.name}`}
						onClick={onEdit}
					>
						<PencilIcon className="size-icon-sm" />
					</Button>
					<Button
						variant="outline"
						size="icon"
						aria-label={`Remove allotment for ${entry.name}`}
						onClick={onRemove}
					>
						<TrashIcon className="size-icon-sm" />
					</Button>
				</div>
			</TableCell>
		</TableRow>
	);
};

type UsageRowsProps = {
	usage: AllotmentUsage;
	unallottedBps: number;
	unallottedHours: number | undefined;
};

const UsageRows: React.FC<UsageRowsProps> = ({
	usage,
	unallottedBps,
	unallottedHours,
}) => {
	const notAttributedMs = usage.notAttributedMs ?? 0;
	return (
		<>
			{usage.withoutAllotment.map((entry) => (
				<UsageOnlyRow
					key={entry.id}
					name={entry.name}
					href={entry.href}
					usedMs={entry.usedMs}
				/>
			))}
			{notAttributedMs > 0 && (
				<UsageOnlyRow
					name="Not attributed"
					info={
						<InfoTooltip size="small" ariaLabel="About hours not attributed">
							These hours ran in chats that were deleted before Coder tracked
							usage per organization.
						</InfoTooltip>
					}
					usedMs={notAttributedMs}
				/>
			)}
			<TableRow>
				<TargetNameCell
					name={usage.remainderLabel}
					href={usage.remainderHref}
				/>
				<AllotmentCell bps={unallottedBps} hours={unallottedHours} />
				<UsedCell
					usedMs={usage.remainderUsedMs}
					rowName={usage.remainderLabel}
					allottedHours={unallottedHours}
				/>
				<TableCell />
			</TableRow>
		</>
	);
};

type TargetNameCellProps = {
	name: string;
	href?: string;
	info?: React.ReactNode;
};

const TargetNameCell: React.FC<TargetNameCellProps> = ({
	name,
	href,
	info,
}) => {
	const label = href ? (
		<Link
			asChild
			size="sm"
			showExternalIcon={false}
			// The default left padding offsets linked names from the unlinked
			// names in the other rows.
			className="pl-0"
		>
			<RouterLink to={href}>{name}</RouterLink>
		</Link>
	) : (
		name
	);
	return (
		<TableCell className="font-medium text-content-primary wrap-anywhere">
			{info ? (
				<div className="flex items-center gap-1">
					{label}
					{info}
				</div>
			) : (
				label
			)}
		</TableCell>
	);
};

type UsageOnlyRowProps = TargetNameCellProps & { usedMs: number };

const UsageOnlyRow: React.FC<UsageOnlyRowProps> = ({
	usedMs,
	...nameProps
}) => (
	<TableRow>
		<TargetNameCell {...nameProps} />
		<TableCell className="text-content-secondary">None</TableCell>
		<UsedCell usedMs={usedMs} rowName={nameProps.name} />
		<TableCell />
	</TableRow>
);

type AllotmentCellProps = {
	bps: number;
	hours: number | undefined;
};

const AllotmentCell: React.FC<AllotmentCellProps> = ({ bps, hours }) => (
	<TableCell className="text-content-primary">
		{formatAllotmentPercent(bps)}
		{hours !== undefined && (
			<div className="text-content-secondary">{formatHours(hours)}</div>
		)}
	</TableCell>
);

type UsedCellProps = {
	usedMs: number;
	rowName: string;
	allottedHours?: number;
};

const UsedCell: React.FC<UsedCellProps> = ({
	usedMs,
	rowName,
	allottedHours,
}) => {
	const hours = usedAgentHours(usedMs);
	return (
		<TableCell className="text-content-primary">
			<div className="flex min-w-16 flex-col gap-1.5">
				<span className="whitespace-nowrap tabular-nums">
					{formatUsedAgentHours(usedMs)} hours
				</span>
				{allottedHours !== undefined && (
					<UsageBar
						percent={usageProgressPercentage(hours, allottedHours)}
						severity={getSeverity(hours, allottedHours)}
						ariaLabel={`Agent Hours used by ${rowName}`}
					/>
				)}
			</div>
		</TableCell>
	);
};
