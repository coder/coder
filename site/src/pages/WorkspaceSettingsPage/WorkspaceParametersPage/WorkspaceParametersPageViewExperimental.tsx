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
	const autofillByName = Object.fromEntries(
		autofillParameters.map((param) => [param.name, param]),
	);
	const initialTouched = parameters.reduce(
		(touched, parameter) => {
			if (autofillByName[parameter.name] !== undefined) {
				touched[parameter.name] = true;
			}
			return touched;
		},
		{} as Record<string, boolean>,
	);
	const form = useFormik({
		onSubmit,
		initialValues: {
			rich_parameter_values: getInitialParameterValues(
				parameters,
				autofillParameters,
			),
		},
		initialTouched,
		validationSchema: useValidationSchemaForDynamicParameters(parameters),
		enableReinitialize: false,
		validateOnChange: true,
		validateOnBlur: true,
	});
	// Group parameters by ephemeral status
	const ephemeralParameters = parameters.filter((p) => p.ephemeral);
	const standardParameters = parameters.filter((p) => !p.ephemeral);

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
		form.setFieldTouched(parameter.name, true);
		sendDynamicParamsRequest(parameter, value);
	};

	// Send the changed parameter and all touched parameters to the websocket
	const sendDynamicParamsRequest = (
		parameter: PreviewParameter,
		value: string,
	) => {
		const formInputs: Record<string, string> = {};
		formInputs[parameter.name] = value;
		const parameters = form.values.rich_parameter_values ?? [];

		for (const [fieldName, isTouched] of Object.entries(form.touched)) {
			if (isTouched && fieldName !== parameter.name) {
				const param = parameters.find((p) => p.name === fieldName);
				if (param?.value) {
					formInputs[fieldName] = param.value;
				}
			}
		}

		sendMessage(formInputs);
	};

	useSyncFormParameters({
		parameters,
		formValues: form.values.rich_parameter_values ?? [],
		setFieldValue: form.setFieldValue,
	});

	return (
		<>
			{disabled && (
				<Alert severity="warning" className="mb-8">
					The template for this workspace requires automatic updates. Update the
					workspace to edit parameters.
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
					<p className="m-0 text-sm font-medium">
						{templateVersionId ?? workspace.latest_build.template_version_id}
					</p>
				</div>
			)}

			<form onSubmit={form.handleSubmit} className="flex flex-col gap-8">
				{standardParameters.length > 0 && (
					<section className="flex flex-col gap-9">
						<hgroup>
							<h2 className="text-xl font-medium mb-0">Parameters</h2>
							<p className="text-sm text-content-secondary m-0">
								These are the settings used by your template. Immutable
								parameters cannot be modified once the workspace is created.
								<Link
									href={docs(
										"/admin/templates/extending-templates/parameters#enable-dynamic-parameters-early-access",
									)}
								>
									View docs
								</Link>
							</p>
						</hgroup>
						{standardParameters.map((parameter, index) => {
							const parameterField = `rich_parameter_values.${index}`;
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
									value={
										form.values?.rich_parameter_values?.[index]?.value || ""
									}
								/>
							);
						})}
					</section>
				)}

				{ephemeralParameters.length > 0 && (
					<section className="flex flex-col gap-6">
						<hgroup>
							<h2 className="text-xl font-medium mb-1">Ephemeral Parameters</h2>
							<p className="text-sm text-content-secondary m-0">
								These parameters only apply for a single workspace start
							</p>
						</hgroup>

						<div className="flex flex-col gap-9">
							{ephemeralParameters.map((parameter, index) => {
								const actualIndex = standardParameters.length + index;
								const parameterField = `rich_parameter_values.${actualIndex}`;
								const isDisabled =
									disabled || parameter.styling?.disabled || isSubmitting;

								return (
									<DynamicParameter
										key={parameter.name}
										parameter={parameter}
										onChange={(value) =>
											handleChange(parameter, parameterField, value)
										}
										autofill={false}
										disabled={isDisabled}
										value={
											form.values?.rich_parameter_values?.[index]?.value || ""
										}
									/>
								);
							})}
						</div>
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
