import {
	API,
	type CreateTaskFeedbackRequest,
	type TaskFeedbackRating,
} from "api/api";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import type { DialogProps } from "components/Dialogs/Dialog";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
import { useFormik } from "formik";
import { FrownIcon, MehIcon, SmileIcon } from "lucide-react";
import type { FC, HTMLProps, ReactNode } from "react";
import { useMutation } from "react-query";

type TaskFeedbackFormValues = {
	rate: TaskFeedbackRating | null;
	comment: string;
};

type TaskFeedbackDialogProps = DialogProps & {
	taskId: string;
};

export const TaskFeedbackDialog: FC<TaskFeedbackDialogProps> = ({
	taskId,
	...dialogProps
}) => {
	const {
		mutate: createFeedback,
		error,
		isPending,
	} = useMutation({
		mutationFn: (req: CreateTaskFeedbackRequest) =>
			API.createTaskFeedback(taskId, req),
		onSuccess: () => {
			displaySuccess("Feedback submitted successfully");
		},
	});

	const formik = useFormik<TaskFeedbackFormValues>({
		initialValues: {
			rate: null,
			comment: "",
		},
		onSubmit: (values) => {
			if (values.rate !== null) {
				createFeedback({
					rate: values.rate,
					comment: values.comment,
				});
			}
		},
	});

	const isRateSelected = Boolean(formik.values.rate);

	return (
		<Dialog {...dialogProps}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Task feedback</DialogTitle>
					<DialogDescription>
						Your feedback is important to us. Please rate your experience with
						this task.
					</DialogDescription>
				</DialogHeader>

				<form
					id="feedback-form"
					onSubmit={formik.handleSubmit}
					className="flex flex-col gap-4"
				>
					{error && <ErrorAlert error={error} />}

					<fieldset className="flex flex-col gap-1">
						<legend className="sr-only">Rate your experience</legend>
						<RateOption {...formik.getFieldProps("rate")} value="good">
							<SmileIcon />I achieved my goal
						</RateOption>
						<RateOption {...formik.getFieldProps("rate")} value="okay">
							<MehIcon />
							It sort of worked, but struggled a lot
						</RateOption>
						<RateOption {...formik.getFieldProps("rate")} value="bad">
							<FrownIcon />
							It was a flop
						</RateOption>
					</fieldset>

					<label className="sr-only" htmlFor="comment">
						Additional comments
					</label>
					<Textarea
						id="comment"
						placeholder="Wanna say something else?..."
						className="h-32 resize-none"
						{...formik.getFieldProps("comment")}
					/>
				</form>

				<DialogFooter>
					<DialogClose asChild>
						<Button variant="outline">Close</Button>
					</DialogClose>
					<Button
						type="submit"
						form="feedback-form"
						disabled={!isRateSelected || isPending}
					>
						<Spinner loading={isPending} />
						Submit Feedback
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type RateOptionProps = HTMLProps<HTMLInputElement> & {
	children: ReactNode;
};

const RateOption: FC<RateOptionProps> = ({ children, ...inputProps }) => {
	return (
		<label
			className={`
			cursor-pointer border border-border border-solid hover:bg-surface-secondary
			px-4 py-3 rounded text-sm has-[:checked]:bg-surface-quaternary
			flex items-center gap-3 [&_svg]:size-4
		`}
		>
			<input className="hidden" type="radio" {...inputProps} />
			{children}
		</label>
	);
};
