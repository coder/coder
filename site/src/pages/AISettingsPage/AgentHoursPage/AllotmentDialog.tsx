import { useId, useState } from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	allotmentHours,
	formatAllotmentPercent,
	formatHours,
	parseAllotmentPercent,
} from "./allotments";

export type AllotmentTarget = {
	id: string;
	name: string;
};

type AllotmentDialogProps = {
	onClose: () => void;
	entity: "organization" | "group";
	/** The allotment being edited. When omitted, the user picks a candidate. */
	target?: AllotmentTarget & { bps: number };
	candidates: readonly AllotmentTarget[];
	/** The largest share the target may hold, in basis points. */
	availableBps: number;
	/** Undefined when the pool size is unknown. */
	poolHours: number | undefined;
	onSubmit: (targetId: string, bps: number) => Promise<unknown>;
};

const validatePercent = (
	value: string,
	availableBps: number,
): { bps?: number; error?: string } => {
	if (value.trim() === "") {
		return { error: "Enter a percentage." };
	}
	if (Number(value) < 0) {
		return { error: "Enter a percentage above 0." };
	}
	const bps = parseAllotmentPercent(value);
	if (bps === undefined) {
		return { error: "Enter a number with at most two decimals." };
	}
	if (bps <= 0) {
		return { error: "Enter a percentage above 0." };
	}
	if (bps > availableBps) {
		return {
			error: `Only ${formatAllotmentPercent(availableBps)} is available.`,
		};
	}
	return { bps };
};

/** Mounted only while open, so its state starts fresh for every edit. */
export const AllotmentDialog: React.FC<AllotmentDialogProps> = ({
	onClose,
	entity,
	target,
	candidates,
	availableBps,
	poolHours,
	onSubmit,
}) => {
	const targetId = useId();
	const percentId = useId();
	const [selectedId, setSelectedId] = useState(target?.id ?? "");
	const [percent, setPercent] = useState(
		target ? String(target.bps / 100) : "",
	);
	const [showValidation, setShowValidation] = useState(false);
	const [isSubmitting, setIsSubmitting] = useState(false);
	const [submitError, setSubmitError] = useState<unknown>(null);

	const validation = validatePercent(percent, availableBps);
	const validationError = showValidation ? validation.error : undefined;
	const targetError =
		showValidation && selectedId === ""
			? `Select ${entity === "group" ? "a group" : "an organization"}.`
			: undefined;
	const availableHours = allotmentHours(availableBps, poolHours);
	const entityLabel = entity === "group" ? "Group" : "Organization";

	const handleSubmit = (event: React.FormEvent) => {
		event.preventDefault();
		setShowValidation(true);
		if (validation.bps === undefined || selectedId === "") {
			return;
		}
		setIsSubmitting(true);
		setSubmitError(null);
		onSubmit(selectedId, validation.bps).then(
			() => {
				setIsSubmitting(false);
				onClose();
			},
			(error: unknown) => {
				setIsSubmitting(false);
				setSubmitError(error);
			},
		);
	};

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open && !isSubmitting) {
					onClose();
				}
			}}
		>
			<DialogContent className="max-w-md">
				<DialogHeader>
					<DialogTitle>
						{target
							? `Edit allotment for ${target.name}`
							: `Add ${entity} allotment`}
					</DialogTitle>
					<DialogDescription>
						Up to {formatAllotmentPercent(availableBps)} is available
						{availableHours !== undefined &&
							` (${formatHours(availableHours)})`}
						.
					</DialogDescription>
				</DialogHeader>
				<form onSubmit={handleSubmit} className="flex flex-col gap-5">
					{submitError != null && (
						<ErrorAlert error={submitError} showDebugDetail={false} />
					)}
					{!target && (
						<div className="flex flex-col gap-2">
							<Label htmlFor={targetId}>{entityLabel}</Label>
							<Select value={selectedId} onValueChange={setSelectedId}>
								<SelectTrigger
									id={targetId}
									aria-invalid={targetError !== undefined}
									aria-describedby={
										targetError ? `${targetId}-error` : undefined
									}
								>
									<SelectValue
										placeholder={`Select ${entity === "group" ? "a group" : "an organization"}`}
									/>
								</SelectTrigger>
								<SelectContent>
									{candidates.map((candidate) => (
										<SelectItem key={candidate.id} value={candidate.id}>
											{candidate.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
							{targetError && (
								<p
									id={`${targetId}-error`}
									className="m-0 text-sm text-content-destructive"
								>
									{targetError}
								</p>
							)}
						</div>
					)}
					<div className="flex flex-col gap-2">
						<Label htmlFor={percentId}>Allotment</Label>
						<InputGroup>
							<InputGroupInput
								id={percentId}
								value={percent}
								onChange={(event) => setPercent(event.target.value)}
								onBlur={() => setShowValidation(true)}
								inputMode="decimal"
								aria-invalid={validationError !== undefined}
								aria-describedby={
									validationError ? `${percentId}-error` : undefined
								}
							/>
							<InputGroupAddon align="inline-end" className="pr-3">
								%
							</InputGroupAddon>
						</InputGroup>
						{validationError && (
							<p
								id={`${percentId}-error`}
								className="m-0 text-sm text-content-destructive"
							>
								{validationError}
							</p>
						)}
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={onClose} disabled={isSubmitting}>
							Cancel
						</Button>
						<Button type="submit" disabled={isSubmitting}>
							<Spinner loading={isSubmitting} />
							Save
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
