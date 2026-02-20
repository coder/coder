import type {
	TemplateVersionParameter,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { FormFields, VerticalForm } from "components/Form/Form";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { useFormik } from "formik";
import type { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import {
	getInitialRichParameterValues,
	useValidationSchemaForRichParameters,
} from "utils/richParameters";
import * as Yup from "yup";

type UpdateBuildParametersDialogProps = {
	open: boolean;
	onClose: () => void;
	onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void;
	missedParameters: TemplateVersionParameter[];
};

export const UpdateBuildParametersDialog: FC<
	UpdateBuildParametersDialogProps
> = ({ missedParameters, onUpdate, open, onClose }) => {
	const form = useFormik({
		initialValues: {
			rich_parameter_values: getInitialRichParameterValues(missedParameters),
		},
		validationSchema: Yup.object({
			rich_parameter_values:
				useValidationSchemaForRichParameters(missedParameters),
		}),
		onSubmit: (values) => {
			onUpdate(values.rich_parameter_values);
		},
		enableReinitialize: true,
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<Dialog
			open={open}
			onOpenChange={(isOpen) => {
				if (!isOpen) onClose();
			}}
		>
			<DialogContent className="max-w-sm" data-testid="dialog">
				<DialogHeader>
					<DialogTitle>Workspace parameters</DialogTitle>
					<DialogDescription>
						This template has new parameters that must be configured to complete
						the update
					</DialogDescription>
				</DialogHeader>

				<VerticalForm onSubmit={form.handleSubmit} id="updateParameters">
					{missedParameters && (
						<FormFields>
							{missedParameters.map((parameter, index) => {
								return (
									<RichParameterInput
										{...getFieldHelpers(
											`rich_parameter_values[${index}].value`,
										)}
										key={parameter.name}
										parameter={parameter}
										onChange={async (value) => {
											await form.setFieldValue(
												`rich_parameter_values.${index}`,
												{
													name: parameter.name,
													value: value,
												},
											);
										}}
									/>
								);
							})}
						</FormFields>
					)}
				</VerticalForm>

				<DialogFooter className="flex-col gap-2">
					<Button
						variant="outline"
						className="w-full"
						type="button"
						onClick={onClose}
					>
						Cancel
					</Button>
					<Button className="w-full" type="submit" form="updateParameters">
						Update parameters
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
