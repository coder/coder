import type {
	TemplateVersionParameter,
	Workspace,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import { ClassicParameterFlowDeprecationWarning } from "modules/workspaces/ClassicParameterFlowDeprecationWarning/ClassicParameterFlowDeprecationWarning";
import type { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import {
	type AutofillBuildParameter,
	getInitialRichParameterValues,
	useValidationSchemaForRichParameters,
} from "utils/richParameters";
import * as Yup from "yup";

export type WorkspaceParametersFormValues = {
	rich_parameter_values: WorkspaceBuildParameter[];
};

interface WorkspaceParameterFormProps {
	workspace: Workspace;
	templateVersionRichParameters: TemplateVersionParameter[];
	autofillParams: AutofillBuildParameter[];
	isSubmitting: boolean;
	canChangeVersions: boolean;
	templatePermissions: { canUpdateTemplate: boolean } | undefined;
	error: unknown;
	onCancel: () => void;
	onSubmit: (values: WorkspaceParametersFormValues) => void;
}

export const WorkspaceParametersForm: FC<WorkspaceParameterFormProps> = ({
	workspace,
	onCancel,
	onSubmit,
	templateVersionRichParameters,
	autofillParams,
	error,
	canChangeVersions,
	templatePermissions,
	isSubmitting,
}) => {
	const form = useFormik<WorkspaceParametersFormValues>({
		onSubmit,
		initialValues: {
			rich_parameter_values: getInitialRichParameterValues(
				templateVersionRichParameters,
				autofillParams,
			),
		},
		validationSchema: Yup.object({
			rich_parameter_values: useValidationSchemaForRichParameters(
				templateVersionRichParameters,
			),
		}),
	});
	const getFieldHelpers = getFormHelpers<WorkspaceParametersFormValues>(
		form,
		error,
	);
	const hasEphemeralParameters = templateVersionRichParameters.some(
		(parameter) => parameter.ephemeral,
	);
	const hasNonEphemeralParameters = templateVersionRichParameters.some(
		(parameter) => !parameter.ephemeral,
	);

	const disabled =
		workspace.outdated &&
		workspace.template_require_active_version &&
		!canChangeVersions;

	return (
		<>
			{disabled && (
				<Alert severity="warning">
					The template for this workspace requires automatic updates. Update the
					workspace to edit parameters.
				</Alert>
			)}
			<ClassicParameterFlowDeprecationWarning
				templateSettingsLink={`/templates/${workspace.organization_name}/${workspace.template_name}/settings`}
				isEnabled={templatePermissions?.canUpdateTemplate ?? false}
			/>
			<HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
				{hasNonEphemeralParameters && (
					<FormSection
						title="Parameters"
						description="Settings used by your template"
					>
						<FormFields>
							{templateVersionRichParameters.map((parameter, index) =>
								// Since we are adding the values to the form based on the index
								// we can't filter them to not loose the right index position
								!parameter.ephemeral ? (
									<RichParameterInput
										{...getFieldHelpers(
											`rich_parameter_values[${index}].value`,
										)}
										disabled={isSubmitting || disabled || !parameter.mutable}
										key={parameter.name}
										onChange={async (value) => {
											await form.setFieldValue(
												`rich_parameter_values.${index}`,
												{
													name: parameter.name,
													value: value,
												},
											);
										}}
										parameter={parameter}
										parameterAutofill={autofillParams?.find(
											({ name }) => name === parameter.name,
										)}
									/>
								) : null,
							)}
						</FormFields>
					</FormSection>
				)}
				{hasEphemeralParameters && (
					<FormSection
						title="Ephemeral Parameters"
						description="These parameters only apply for a single workspace start."
					>
						<FormFields>
							{templateVersionRichParameters.map((parameter, index) =>
								// Since we are adding the values to the form based on the index
								// we can't filter them to not loose the right index position
								parameter.mutable && parameter.ephemeral ? (
									<RichParameterInput
										{...getFieldHelpers(
											`rich_parameter_values[${index}].value`,
										)}
										disabled={isSubmitting || disabled}
										key={parameter.name}
										onChange={async (value) => {
											await form.setFieldValue(
												`rich_parameter_values.${index}`,
												{
													name: parameter.name,
													value: value,
												},
											);
										}}
										parameter={parameter}
									/>
								) : null,
							)}
						</FormFields>
					</FormSection>
				)}

				<FormFooter>
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>

					<Button
						type="submit"
						disabled={isSubmitting || disabled || !form.dirty}
					>
						<Spinner loading={isSubmitting} />
						Submit and restart
					</Button>
				</FormFooter>
			</HorizontalForm>
		</>
	);
};
