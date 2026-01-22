import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import type {
	TemplateVersionParameter,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import type { DialogProps } from "components/Dialogs/Dialog";
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

type UpdateBuildParametersDialogProps = DialogProps & {
	onClose: () => void;
	onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void;
	missedParameters: TemplateVersionParameter[];
};

export const UpdateBuildParametersDialog: FC<
	UpdateBuildParametersDialogProps
> = ({ missedParameters, onUpdate, ...dialogProps }) => {
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
			{...dialogProps}
			scroll="body"
			aria-labelledby="update-build-parameters-title"
			maxWidth="xs"
			data-testid="dialog"
		>
			<DialogTitle
				id="update-build-parameters-title"
				className="py-6 px-10 [&_h2]:text-xl [&_h2]:font-normal"
			>
				Workspace parameters
			</DialogTitle>
			<DialogContent className="px-10">
				<DialogContentText className="m-0">
					This template has new parameters that must be configured to complete
					the update
				</DialogContentText>
				<VerticalForm
					className="pt-8"
					onSubmit={form.handleSubmit}
					id="updateParameters"
				>
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
			</DialogContent>
			<DialogActions disableSpacing className="p-10 flex-col gap-2">
				<Button
					variant="outline"
					className="w-full"
					type="button"
					onClick={dialogProps.onClose}
				>
					Cancel
				</Button>
				<Button className="w-full" type="submit" form="updateParameters">
					Update parameters
				</Button>
			</DialogActions>
		</Dialog>
	);
};
