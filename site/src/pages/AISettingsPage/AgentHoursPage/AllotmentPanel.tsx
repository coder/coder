import { PencilIcon, PlusIcon, TrashIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { getErrorDetail, getErrorStatus } from "#/api/errors";
import { AgentHoursAllotmentMaxBps } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
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
import { AllotmentDialog, type AllotmentTarget } from "./AllotmentDialog";
import {
	allotmentHours,
	formatAllotmentPercent,
	formatHours,
} from "./allotments";

type AllotmentEntry = AllotmentTarget & { bps: number };

type AllotmentPanelProps = {
	entity: "organization" | "group";
	poolLabel: string;
	allotments: readonly AllotmentEntry[] | undefined;
	candidates: readonly AllotmentTarget[];
	/** Undefined when the pool size is unknown. */
	poolHours: number | undefined;
	error: unknown;
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
	onSave,
	onRemove,
}) => {
	const [dialog, setDialog] = useState<DialogState>();
	const [entryToRemove, setEntryToRemove] = useState<AllotmentEntry>();
	const entityLabel = entity === "group" ? "Group" : "Organization";

	if (allotments === undefined) {
		return error != null ? (
			<ErrorAlert error={error} />
		) : (
			<Loader label={`Loading ${entity} allotments`} />
		);
	}

	const allottedBps = allotments.reduce((sum, entry) => sum + entry.bps, 0);
	const unallottedBps = Math.max(AgentHoursAllotmentMaxBps - allottedBps, 0);
	const allottedHours = allotmentHours(allottedBps, poolHours);
	const unallottedHours = allotmentHours(unallottedBps, poolHours);
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
	const handleSave = (id: string, bps: number) =>
		onSave(id, bps).catch((saveError: unknown) => {
			if (getErrorStatus(saveError) !== 404) {
				throw saveError;
			}
			const name =
				[...allotments, ...candidates].find((target) => target.id === id)
					?.name ?? `The ${entity}`;
			toast.error(`${name} is no longer available.`);
		});

	return (
		<div className="flex flex-col gap-4">
			{error != null && <ErrorAlert error={error} />}
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
						<TableHead>
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{allotments.length === 0 ? (
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
								onEdit={() => setDialog({ mode: "edit", entry })}
								onRemove={() => setEntryToRemove(entry)}
							/>
						))
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
	onEdit: () => void;
	onRemove: () => void;
};

const AllotmentRow: React.FC<AllotmentRowProps> = ({
	entry,
	poolHours,
	onEdit,
	onRemove,
}) => {
	const hours = allotmentHours(entry.bps, poolHours);
	return (
		<TableRow>
			<TableCell className="font-medium text-content-primary wrap-anywhere">
				{entry.name}
			</TableCell>
			<TableCell className="text-content-primary">
				{formatAllotmentPercent(entry.bps)}
				{hours !== undefined && (
					<div className="text-content-secondary">{formatHours(hours)}</div>
				)}
			</TableCell>
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
