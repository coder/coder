import type {
	PreviewParameter,
	Workspace,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import { useSyncFormParameters } from "modules/hooks/useSyncFormParameters";
import {
	DynamicParameter,
	getInitialParameterValues,
	useValidationSchemaForDynamicParameters,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import type { FC } from "react";
import { docs } from "utils/docs";
import type { AutofillBuildParameter } from "utils/richParameters";

type WorkspaceParametersPageViewExperimentalProps = {
	workspace: Workspace;
	autofillParameters: AutofillBuildParameter[];
	parameters: PreviewParameter[];
	diagnostics: PreviewParameter["diagnostics"];
	canChangeVersions: boolean;
	isSubmitting: boolean;
	onCancel: () => void;
	onSubmit: (values: {
		rich_parameter_values: WorkspaceBuildParameter[];
	}) => void;
	sendMessage: (formValues: Record<string, string>) => void;
	templateVersionId: string | undefined;
};

export const WorkspaceParametersPageViewExperimental: FC<
	WorkspaceParametersPageViewExperimentalProps
> = ({
	workspace,
	autofillParameters,
	parameters,
	diagnostics,
	canChangeVersions,
	isSubmitting,
	onSubmit,
	sendMessage,
	onCancel,
	templateVersionId,
}) => {
	const form = useFormik({
		onSubmit,
		initialValues: {
			rich_parameter_values: getInitialParameterValues(
				parameters,
				autofillParameters,
			),
		},
		validationSchema: useValidationSchemaForDynamicParameters(parameters),
		enableReinitialize: false,
		validateOnChange: true,
		validateOnBlur: true,
	});

	const disabled =
		workspace.outdated &&
		workspace.template_require_active_version &&
		!canChangeVersions;

	const handleChange = async (
		parameter: PreviewParameter,
		parameterField: string,
		value: string,
	) => {
		await form.setFieldValue(parameterField, {
			name: parameter.name,
			value,
		});
		sendDynamicParamsRequest(parameter, value);
	};

	const sendDynamicParamsRequest = (
		parameter: PreviewParameter,
		value: string,
	) => {
		const formInputs: Record<string, string> = {};
		const parameters = form.values.rich_parameter_values ?? [];
		for (const param of parameters) {
			if (param?.name && param?.value) {
				formInputs[param.name] = param.value;
			}
		}

		formInputs[parameter.name] = value;

		sendMessage(formInputs);
	};

	useSyncFormParameters({
		parameters,
		formValues: form.values.rich_parameter_values ?? [],
		setFieldValue: form.setFieldValue,
	});

	const hasIncompatibleParameters = parameters.some((parameter) => {
		if (!parameter.mutable && parameter.diagnostics.length > 0) {
			return true;
		}
		return false;
	});

	return (
		<>
			{disabled && (
				<Alert severity="warning" className="mb-8">
					The template for this workspace requires automatic updates. Update the
					workspace to edit parameters.
				</Alert>
			)}

			{hasIncompatibleParameters && (
				<Alert severity="error">
					<p className="text-lg leading-tight font-bold m-0">
						Workspace update blocked
					</p>
					<p className="mb-0">
						The new template version includes parameter changes that are
						incompatible with this workspace's existing parameter values. This
						may be caused by:
					</p>
					<ul className="mb-0 pl-4 space-y-1">
						<li>
							New <strong>required</strong> parameters that cannot be provided
							after workspace creation
						</li>
						<li>
							Changes to <strong>valid options or validations</strong> for
							existing parameters
						</li>
						<li>Logic changes that conflict with previously selected values</li>
					</ul>
					<p className="mb-0">
						Please contact the <strong>template administrator</strong> to review
						the changes and ensure compatibility for existing workspaces.
					</p>
					<p className="mb-0">
						Consider supplying defaults for new parameters or validating
						conditional logic against prior workspace states.
					</p>
				</Alert>
			)}

			{diagnostics && diagnostics.length > 0 && (
				<div className="flex flex-col gap-4 mb-8">
					{diagnostics.map((diagnostic, index) => (
						<div
							key={`diagnostic-${diagnostic.summary}-${index}`}
							className={`text-xs flex flex-col rounded-md border px-4 pb-3 border-solid
								${
									diagnostic.severity === "error"
										? " text-content-destructive border-border-destructive"
										: " text-content-warning border-border-warning"
								}`}
						>
							<div className="flex items-center m-0">
								<p className="font-medium">{diagnostic.summary}</p>
							</div>
							{diagnostic.detail && (
								<p className="m-0 pb-0">{diagnostic.detail}</p>
							)}
						</div>
					))}
				</div>
			)}

			{(templateVersionId || workspace.latest_build.template_version_id) && (
				<div className="flex flex-col gap-2">
					<Label className="text-sm text-content-secondary">Version ID</Label>
					<p className="m-0 text-xs font-medium font-mono">
						{templateVersionId ?? workspace.latest_build.template_version_id}
					</p>
				</div>
			)}

			<form onSubmit={form.handleSubmit} className="flex flex-col gap-8">
				{parameters.length > 0 && (
					<section className="flex flex-col gap-9">
						<hgroup>
							<h2 className="text-xl font-medium mb-0">Parameters</h2>
							<p className="text-sm text-content-secondary m-0">
								These are the settings used by your template. Immutable
								parameters cannot be modified once the workspace is created.
								<Link
									href={docs(
										"/admin/templates/extending-templates/dynamic-parameters",
									)}
								>
									View docs
								</Link>
							</p>
						</hgroup>
						{parameters.map((parameter, index) => {
							const currentParameterValueIndex =
								form.values.rich_parameter_values?.findIndex(
									(p) => p.name === parameter.name,
								);
							const parameterFieldIndex =
								currentParameterValueIndex !== undefined
									? currentParameterValueIndex
									: index;
							// Get the form value by parameter name to ensure correct value mapping
							const formValue =
								currentParameterValueIndex !== undefined
									? form.values?.rich_parameter_values?.[
											currentParameterValueIndex
										]?.value || ""
									: "";

							const parameterField = `rich_parameter_values.${parameterFieldIndex}`;
							const isDisabled =
								disabled ||
								parameter.styling?.disabled ||
								!parameter.mutable ||
								isSubmitting;

							return (
								<DynamicParameter
									key={parameter.name}
									parameter={parameter}
									onChange={(value) =>
										handleChange(parameter, parameterField, value)
									}
									autofill={false}
									disabled={isDisabled}
									value={formValue}
								/>
							);
						})}
					</section>
				)}

				<div className="flex justify-end gap-2">
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button
						type="submit"
						disabled={
							isSubmitting ||
							disabled ||
							diagnostics.some(
								(diagnostic) => diagnostic.severity === "error",
							) ||
							parameters.some((parameter) =>
								parameter.diagnostics.some(
									(diagnostic) => diagnostic.severity === "error",
								),
							)
						}
					>
						<Spinner loading={isSubmitting} />
						Update and restart
					</Button>
				</div>
			</form>
		</>
	);
};
