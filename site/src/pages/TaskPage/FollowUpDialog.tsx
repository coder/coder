import type { Task } from "api/typesGenerated";
import { useFormik } from "formik";
import type { FC } from "react";
import { useId } from "react";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Textarea } from "#/components/Textarea/Textarea";

type FollowUpDialogProps = {
	task: Task;
	initialMessage: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onSubmit: (message: string) => void;
};

export const FollowUpDialog: FC<FollowUpDialogProps> = ({
	task,
	initialMessage,
	open,
	onOpenChange,
	onSubmit,
}) => {
	const formId = useId();

	const formik = useFormik({
		initialValues: {
			message: initialMessage,
		},
		enableReinitialize: true,
		onSubmit: (values) => {
			const message = values.message.trim();
			if (message.length === 0) {
				return;
			}
			onSubmit(message);
			onOpenChange(false);
		},
	});

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle>Send Follow-up Message</DialogTitle>
					<DialogDescription>
						Add another message to this task. The task will resume and send this
						follow-up automatically.
					</DialogDescription>
				</DialogHeader>

				<form id={formId} className="space-y-4" onSubmit={formik.handleSubmit}>
					<div>
						<label
							htmlFor={`${formId}-message`}
							className="block text-sm font-medium text-content-primary mb-2"
						>
							Follow-up message
						</label>
						<Textarea
							id={`${formId}-message`}
							name="message"
							value={formik.values.message}
							onChange={formik.handleChange}
							rows={10}
							className="w-full"
							placeholder={`Continue "${task.display_name}" after resume by asking for the next step...`}
						/>
					</div>
				</form>

				<DialogFooter>
					<DialogClose asChild>
						<Button variant="outline">Cancel</Button>
					</DialogClose>
					<Button
						type="submit"
						form={formId}
						disabled={formik.values.message.trim().length === 0}
					>
						Send Follow-up
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
