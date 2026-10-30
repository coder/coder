import { useFormik } from "formik";
import { useId } from "react";
import { isApiValidationError, mapApiErrorToFieldErrors } from "#/api/errors";
import type { CreateChatProjectTriggerRequest } from "#/api/typesGenerated";
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
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { useTime } from "#/hooks/useTime";
import { getPreferredTimezone } from "#/utils/timeZones";
import { FieldHint } from "./TriggerChipEditors";
import { TriggerScheduleFields } from "./TriggerScheduleFields";
import { triggerPromptMaxLength } from "./triggerCatalog";
import {
	draftFromSchedule,
	type ScheduleDraft,
	scheduleFromDraft,
	validateScheduleDraft,
} from "./triggerSchedule";

/** What a new schedule trigger needs: when it runs and what it asks. */
type NewScheduleTrigger = Required<
	Pick<CreateChatProjectTriggerRequest, "schedule" | "prompt">
>;

type TriggerScheduleDialogProps = {
	readonly agentName: string;
	readonly isCreating: boolean;
	/** The latest failed create. Field errors show on their fields. */
	readonly error: unknown;
	/**
	 * The time the next-run preview counts from. Unset uses the current
	 * time; stories pin it.
	 */
	readonly now?: Date;
	/** The viewer's time zone, the default for new schedules. Unset uses the browser's. */
	readonly timeZone?: string;
	/** Called when the schedule changes, so a rejection of the old one can be cleared. */
	readonly onScheduleChange: () => void;
	readonly onClose: () => void;
	readonly onCreate: (trigger: NewScheduleTrigger) => void;
};

type ScheduleTriggerValues = ScheduleDraft & { readonly prompt: string };

/**
 * Creates a schedule trigger. Unlike webhook triggers, it needs a schedule
 * and a prompt before it can be saved, since a run has no other input.
 * Mount it to open it; it starts empty each time.
 */
export const TriggerScheduleDialog: React.FC<TriggerScheduleDialogProps> = ({
	agentName,
	isCreating,
	error,
	now,
	timeZone,
	onScheduleChange,
	onClose,
	onCreate,
}) => {
	const promptId = useId();
	const liveNow = useTime(() => new Date(), {
		interval: 30_000,
		disabled: now !== undefined,
	});
	const currentTime = now ?? liveNow;
	const viewerTimeZone = timeZone ?? getPreferredTimezone();
	const form = useFormik<ScheduleTriggerValues>({
		initialValues: {
			...draftFromSchedule(undefined, viewerTimeZone),
			prompt: "",
		},
		onSubmit: (values) =>
			onCreate({
				schedule: scheduleFromDraft(values),
				prompt: values.prompt.trim(),
			}),
	});
	const scheduleErrors = validateScheduleDraft(form.values, currentTime);
	const fieldErrors = isApiValidationError(error)
		? mapApiErrorToFieldErrors(error.response.data)
		: {};
	const hasFieldError = Boolean(fieldErrors.schedule || fieldErrors.prompt);
	const canCreate =
		!isCreating &&
		Object.keys(scheduleErrors).length === 0 &&
		form.values.prompt.trim() !== "";

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open && !isCreating) {
					onClose();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Add a schedule trigger</DialogTitle>
					<DialogDescription>
						Start a chat with {agentName} on a schedule. You can also run it any
						time from its menu.
					</DialogDescription>
				</DialogHeader>
				<form
					className="flex flex-col gap-4"
					onSubmit={(event) => {
						event.preventDefault();
						if (canCreate) {
							form.handleSubmit();
						}
					}}
				>
					<TriggerScheduleFields
						draft={form.values}
						errors={scheduleErrors}
						serverError={fieldErrors.schedule}
						now={currentTime}
						timeZone={viewerTimeZone}
						onChange={(draft) => {
							void form.setValues({ ...form.values, ...draft });
							onScheduleChange();
						}}
					/>
					<div className="flex flex-col gap-1.5">
						<Label htmlFor={promptId}>Prompt</Label>
						<Textarea
							id={promptId}
							name="prompt"
							rows={4}
							value={form.values.prompt}
							maxLength={triggerPromptMaxLength}
							placeholder="Summarize the pull requests merged since the last run."
							aria-invalid={Boolean(fieldErrors.prompt)}
							aria-describedby={`${promptId}-hint`}
							onChange={form.handleChange}
						/>
						<FieldHint id={`${promptId}-hint`} error={fieldErrors.prompt}>
							What {agentName} does on each run.
						</FieldHint>
					</div>
					{Boolean(error) && !hasFieldError && <ErrorAlert error={error} />}
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							disabled={isCreating}
							onClick={onClose}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={!canCreate}>
							<Spinner loading={isCreating} />
							Add trigger
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
