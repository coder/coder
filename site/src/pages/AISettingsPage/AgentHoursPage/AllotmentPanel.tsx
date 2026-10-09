import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { getErrorDetail } from "#/api/errors";
import { AgentHoursAllotmentMaxBps } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
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
	const addBlockedReason =
		unallottedBps === 0
			? `All of ${poolLabel} are allotted.`
			: candidates.length === 0
				? `Every ${entity} has an allotment.`
				: undefined;

	const handleRemove = (entry: AllotmentEntry) => {
		toast.promise(onRemove(entry.id), {
			loading: `Removing allotment for ${entry.name}...`,
			success: `Removed allotment for ${entry.name}.`,
			error: (removeError) => ({
				message: `Failed to remove allotment for ${entry.name}.`,
				description: getErrorDetail(removeError),
			}),
		});
	};

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
					</span>
					, {formatAllotmentPercent(unallottedBps)} unallotted
					{allottedHours !== undefined &&
						poolHours !== undefined &&
						` (${formatHours(allottedHours)} of ${formatHours(poolHours)})`}
				</p>
			</div>

			<Table aria-label={`${entityLabel} allotments`}>
				<TableHeader>
					<TableRow>
						<TableHead>{entityLabel}</TableHead>
						<TableHead>Allotment</TableHead>
						{poolHours !== undefined && <TableHead>Hours</TableHead>}
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
								onRemove={() => handleRemove(entry)}
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
					availableBps={
						dialog.mode === "edit"
							? unallottedBps + dialog.entry.bps
							: unallottedBps
					}
					poolHours={poolHours}
					onSubmit={onSave}
				/>
			)}
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
			<TableCell className="font-medium text-content-primary">
				{entry.name}
			</TableCell>
			<TableCell>{formatAllotmentPercent(entry.bps)}</TableCell>
			{hours !== undefined && <TableCell>{formatHours(hours)}</TableCell>}
			<TableCell>
				<div className="flex justify-end gap-2">
					<Button
						variant="outline"
						size="sm"
						aria-label={`Edit allotment for ${entry.name}`}
						onClick={onEdit}
					>
						Edit
					</Button>
					<Button
						variant="subtle"
						size="sm"
						aria-label={`Remove allotment for ${entry.name}`}
						onClick={onRemove}
					>
						Remove
					</Button>
				</div>
			</TableCell>
		</TableRow>
	);
};
