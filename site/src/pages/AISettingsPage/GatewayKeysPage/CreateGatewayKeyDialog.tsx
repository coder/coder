import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import type { CreateAIGatewayKeyResponse } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers } from "#/utils/formUtils";

interface CreateGatewayKeyFormValues {
	name: string;
}

const validationSchema = Yup.object({
	name: Yup.string()
		.required("Name is required.")
		.matches(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, {
			excludeEmptyString: true,
			message:
				"Use lowercase letters and numbers with optional single hyphens between words.",
		})
		.max(64, "Name cannot be longer than 64 characters."),
});

interface CreateGatewayKeyDialogProps {
	open: boolean;
	onClose: () => void;
	onCreate: (name: string) => void;
	createdKey?: CreateAIGatewayKeyResponse;
	submitError?: unknown;
	isSubmitting?: boolean;
}

export const CreateGatewayKeyDialog: FC<CreateGatewayKeyDialogProps> = ({
	open,
	onClose,
	onCreate,
	createdKey,
	submitError,
	isSubmitting = false,
}) => {
	const form = useFormik<CreateGatewayKeyFormValues>({
		initialValues: { name: "" },
		validationSchema,
		onSubmit: (values) => {
			onCreate(values.name);
		},
	});
	const getFieldHelpers = getFormHelpers(form, submitError);

	const closeDialog = () => {
		form.resetForm();
		onClose();
	};

	const isBusy = isSubmitting;
	const showSubmitError =
		Boolean(submitError) && !isApiValidationError(submitError);

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen && !isBusy && !createdKey) {
					closeDialog();
				}
			}}
		>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto"
				aria-describedby={undefined}
			>
				<DialogHeader>
					<DialogTitle>
						{createdKey ? "Save your AI Gateway key" : "Create AI Gateway key"}
					</DialogTitle>
				</DialogHeader>

				{createdKey ? (
					<div className="flex flex-col gap-5">
						<Alert severity="warning">
							<AlertDescription>
								Copy this key now. For security reasons it cannot be shown
								again.
							</AlertDescription>
						</Alert>
						<CodeExample
							secret={false}
							code={createdKey.key}
							className="min-h-0 select-all w-full"
						/>
						<DialogFooter>
							<Button onClick={closeDialog}>Done</Button>
						</DialogFooter>
					</div>
				) : (
					<div className="flex flex-col gap-5">
						{showSubmitError && <ErrorAlert error={submitError} />}
						<form onSubmit={form.handleSubmit} className="flex flex-col gap-5">
							<FormField
								field={getFieldHelpers("name", {
									helperText:
										"Lowercase letters and numbers with optional single hyphens between words. Maximum 64 characters.",
								})}
								label="Name"
								placeholder="primary-gateway"
								autoFocus
								autoComplete="off"
							/>
							<DialogFooter>
								<Button
									variant="outline"
									disabled={isBusy}
									onClick={closeDialog}
								>
									Cancel
								</Button>
								<Button type="submit" disabled={isBusy || !form.dirty}>
									<Spinner loading={isBusy} />
									Create
								</Button>
							</DialogFooter>
						</form>
					</div>
				)}
			</DialogContent>
		</Dialog>
	);
};
