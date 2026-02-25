import { getErrorMessage } from "api/errors";
import { resumeTask, sendTaskInput } from "api/queries/tasks";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import type { Task, Workspace } from "api/typesGenerated";
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
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
import { useFormik } from "formik";
import type { FC } from "react";
import { useEffect, useId, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";

type FollowUpDialogProps = {
	task: Task;
	workspace: Workspace;
	open: boolean;
	onOpenChange: (open: boolean) => void;
};

export const FollowUpDialog: FC<FollowUpDialogProps> = ({
	task,
	workspace,
	open,
	onOpenChange,
}) => {
	const formId = useId();
	const queryClient = useQueryClient();
	const [flowError, setFlowError] = useState<string>();
	const [stage, setStage] = useState<"idle" | "sending" | "resuming">("idle");

	const sendInputMutation = useMutation(sendTaskInput(task, queryClient));
	const resumeMutation = useMutation(resumeTask(task, queryClient));

	const formik = useFormik({
		initialValues: {
			message: "",
		},
		onSubmit: async (values) => {
			const message = values.message.trim();
			if (message.length === 0) {
				return;
			}

			setFlowError(undefined);

			// If the task is already active, send immediately.
			if (task.status === "active") {
				setStage("sending");
				try {
					await sendInputMutation.mutateAsync(message);
					formik.resetForm();
					setStage("idle");
					onOpenChange(false);
					return;
				} catch (error) {
					setFlowError(getErrorMessage(error, "Failed to send message."));
					setStage("idle");
					return;
				}
			}

			// Otherwise, resume first and wait for polling to report active.
			setStage("resuming");
			try {
				await resumeMutation.mutateAsync();
				await queryClient.invalidateQueries({
					queryKey: ["tasks", task.owner_name, task.id],
				});
				await queryClient.invalidateQueries({
					queryKey: workspaceByOwnerAndNameKey(
						workspace.owner_name,
						workspace.name,
					),
				});
			} catch (error) {
				setFlowError(getErrorMessage(error, "Failed to resume task."));
				setStage("idle");
			}
		},
	});

	useEffect(() => {
		if (!open) {
			setFlowError(undefined);
			setStage("idle");
			formik.resetForm();
			return;
		}
	}, [open, formik.resetForm]);

	useEffect(() => {
		if (stage !== "resuming") {
			return;
		}

		if (
			workspace.latest_build.status === "failed" ||
			workspace.latest_build.status === "canceled"
		) {
			setFlowError(
				"Failed to resume task because the workspace build did not complete successfully.",
			);
			setStage("idle");
			return;
		}

		if (task.status !== "active") {
			return;
		}

		const sendAfterResume = async () => {
			const message = formik.values.message.trim();
			if (!message) {
				setStage("idle");
				return;
			}
			setStage("sending");
			try {
				await sendInputMutation.mutateAsync(message);
				formik.resetForm();
				onOpenChange(false);
				setStage("idle");
			} catch (error) {
				setFlowError(getErrorMessage(error, "Failed to send message."));
				setStage("idle");
			}
		};

		void sendAfterResume();
	}, [
		stage,
		task.status,
		workspace.latest_build.status,
		sendInputMutation,
		formik.values.message,
		onOpenChange,
		formik.resetForm,
	]);

	const isWorking = stage !== "idle";
	const buildLogsHref = `/@${workspace.owner_name}/${workspace.name}/builds/${workspace.latest_build.build_number}`;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle>Send Follow-up Message</DialogTitle>
					<DialogDescription>
						Resume this task and send an additional message to the existing
						session.
					</DialogDescription>
				</DialogHeader>

				<form id={formId} className="space-y-4" onSubmit={formik.handleSubmit}>
					{flowError && <ErrorAlert error={flowError} />}

					{(stage === "resuming" || stage === "sending") && (
						<div className="rounded-md border border-border p-3 text-sm">
							<div className="flex items-center gap-2 text-content-primary">
								<Spinner loading />
								<span>
									{stage === "resuming"
										? "Resuming task..."
										: "Sending follow-up message..."}
								</span>
							</div>
							<p className="m-0 mt-2 text-content-secondary">
								Build status: <strong>{workspace.latest_build.status}</strong>
							</p>
							<Button variant="subtle" size="sm" asChild className="mt-2">
								<RouterLink to={buildLogsHref}>View build logs</RouterLink>
							</Button>
						</div>
					)}

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
							disabled={isWorking}
							className="w-full"
							placeholder={`Continue "${task.display_name}" after resume by asking for the next step...`}
						/>
					</div>
				</form>

				<DialogFooter>
					<DialogClose asChild>
						<Button variant="outline" disabled={isWorking}>
							Cancel
						</Button>
					</DialogClose>
					<Button
						type="submit"
						form={formId}
						disabled={formik.values.message.trim().length === 0 || isWorking}
					>
						<Spinner loading={isWorking} />
						Resume and Send Message
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
