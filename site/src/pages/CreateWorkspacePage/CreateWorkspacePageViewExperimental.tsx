import type * as TypesGen from "api/typesGenerated";
import type {
	DynamicParametersRequest,
	PreviewDiagnostics,
	PreviewParameter,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { SelectFilter } from "components/Filter/SelectFilter";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Pill } from "components/Pill/Pill";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { Switch } from "components/Switch/Switch";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FormikContextType, useFormik } from "formik";
import { useDebouncedFunction } from "hooks/debounce";
import { ArrowLeft } from "lucide-react";
import {
	DynamicParameter,
	getInitialParameterValues,
	useValidationSchemaForDynamicParameters,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import {
	type FC,
	useCallback,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";
import { getFormHelpers, nameValidator } from "utils/formUtils";
import type { AutofillBuildParameter } from "utils/richParameters";
import * as Yup from "yup";
import type {
	CreateWorkspaceMode,
	ExternalAuthPollingState,
} from "./CreateWorkspacePage";
import { ExternalAuthButton } from "./ExternalAuthButton";
import type { CreateWorkspacePermissions } from "./permissions";

export interface CreateWorkspacePageViewExperimentalProps {
	autofillParameters: AutofillBuildParameter[];
	creatingWorkspace: boolean;
	defaultName?: string | null;
	defaultOwner: TypesGen.User;
	diagnostics: PreviewDiagnostics;
	disabledParams?: string[];
	error: unknown;
	externalAuth: TypesGen.TemplateVersionExternalAuth[];
	externalAuthPollingState: ExternalAuthPollingState;
	hasAllRequiredExternalAuth: boolean;
	mode: CreateWorkspaceMode;
	parameters: PreviewParameter[];
	permissions: CreateWorkspacePermissions;
	presets: TypesGen.Preset[];
	template: TypesGen.Template;
	versionId?: string;
	onCancel: () => void;
	onSubmit: (
		req: TypesGen.CreateWorkspaceRequest,
		owner: TypesGen.User,
	) => void;
	resetMutation: () => void;
	sendMessage: (message: Record<string, string>) => void;
	startPollingExternalAuth: () => void;
}

export const CreateWorkspacePageViewExperimental: FC<
	CreateWorkspacePageViewExperimentalProps
> = ({
	autofillParameters,
	creatingWorkspace,
	defaultName,
	defaultOwner,
	diagnostics,
	disabledParams,
	error,
	externalAuth,
	externalAuthPollingState,
	hasAllRequiredExternalAuth,
	mode,
	parameters,
	permissions,
	presets = [],
	template,
	versionId,
	onSubmit,
	onCancel,
	resetMutation,
	sendMessage,
	startPollingExternalAuth,
}) => {
	const [owner, setOwner] = useState(defaultOwner);
	const [suggestedName, setSuggestedName] = useState(() =>
		generateWorkspaceName(),
	);
	const [showPresetParameters, setShowPresetParameters] = useState(false);
	const id = useId();
	const rerollSuggestedName = useCallback(() => {
		setSuggestedName(() => generateWorkspaceName());
	}, []);

	const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
		useFormik<TypesGen.CreateWorkspaceRequest>({
			initialValues: {
				name: defaultName ?? "",
				template_id: template.id,
				rich_parameter_values: getInitialParameterValues(
					parameters,
					autofillParameters,
				),
			},
			validationSchema: Yup.object({
				name: nameValidator("Workspace Name"),
				rich_parameter_values:
					useValidationSchemaForDynamicParameters(parameters),
			}),
			enableReinitialize: true,
			validateOnChange: false,
			validateOnBlur: true,
			onSubmit: (request) => {
				if (!hasAllRequiredExternalAuth) {
					return;
				}

				onSubmit(request, owner);
			},
		});

	useEffect(() => {
		if (error) {
			window.scrollTo(0, 0);
		}
	}, [error]);

	const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
		form,
		error,
	);

	const autofillByName = useMemo(
		() =>
			Object.fromEntries(
				autofillParameters.map((param) => [param.name, param]),
			),
		[autofillParameters],
	);

	const [presetOptions, setPresetOptions] = useState([
		{ label: "None", value: "" },
	]);
	useEffect(() => {
		setPresetOptions([
			{ label: "None", value: "" },
			...presets.map((preset) => ({
				label: preset.Name,
				value: preset.ID,
			})),
		]);
	}, [presets]);

	const [selectedPresetIndex, setSelectedPresetIndex] = useState(0);
	const [presetParameterNames, setPresetParameterNames] = useState<string[]>(
		[],
	);

	useEffect(() => {
		const selectedPresetOption = presetOptions[selectedPresetIndex];
		let selectedPreset: TypesGen.Preset | undefined;
		for (const preset of presets) {
			if (preset.ID === selectedPresetOption.value) {
				selectedPreset = preset;
				break;
			}
		}

		if (!selectedPreset || !selectedPreset.Parameters) {
			setPresetParameterNames([]);
			return;
		}

		setPresetParameterNames(selectedPreset.Parameters.map((p) => p.Name));

		for (const presetParameter of selectedPreset.Parameters) {
			const parameterIndex = parameters.findIndex(
				(p) => p.name === presetParameter.Name,
			);
			if (parameterIndex === -1) continue;

			const parameterField = `rich_parameter_values.${parameterIndex}`;

			form.setFieldValue(parameterField, {
				name: presetParameter.Name,
				value: presetParameter.Value,
			});
		}
	}, [
		presetOptions,
		selectedPresetIndex,
		presets,
		form.setFieldValue,
		parameters,
	]);

	const sendDynamicParamsRequest = (
		parameter: PreviewParameter,
		value: string,
	) => {
		const formInputs = Object.fromEntries(
			form.values.rich_parameter_values?.map((value) => {
				return [value.name, value.value];
			}) ?? [],
		);
		// Update the input for the changed parameter
		formInputs[parameter.name] = value;

		sendMessage(formInputs);
	};

	const { debounced: handleChangeDebounced } = useDebouncedFunction(
		async (
			parameter: PreviewParameter,
			parameterField: string,
			value: string,
		) => {
			await form.setFieldValue(parameterField, {
				name: parameter.name,
				value,
			});
			sendDynamicParamsRequest(parameter, value);
		},
		500,
	);

	const handleChange = async (
		parameter: PreviewParameter,
		parameterField: string,
		value: string,
	) => {
		if (parameter.form_type === "input" || parameter.form_type === "textarea") {
			handleChangeDebounced(parameter, parameterField, value);
		} else {
			await form.setFieldValue(parameterField, {
				name: parameter.name,
				value,
			});
			sendDynamicParamsRequest(parameter, value);
		}
	};

	return (
		<>
			<div className="absolute sticky top-5 ml-10">
				<button
					onClick={onCancel}
					type="button"
					className="flex items-center gap-2 bg-transparent border-none text-content-secondary hover:text-content-primary translate-y-12"
				>
					<ArrowLeft size={20} />
					Go back
				</button>
			</div>
			<div className="flex flex-col gap-6 max-w-screen-sm mx-auto">
				<header className="flex flex-col gap-2 mt-10">
					<div className="flex items-center gap-2">
						<Avatar
							variant="icon"
							size="md"
							src={template.icon}
							fallback={template.name}
						/>
						<p className="text-base font-medium m-0">
							{template.display_name.length > 0
								? template.display_name
								: template.name}
						</p>
					</div>
					<h1 className="text-3xl font-semibold m-0">New workspace</h1>

					{template.deprecated && <Pill type="warning">Deprecated</Pill>}
				</header>

				<form
					onSubmit={form.handleSubmit}
					aria-label="Create workspace form"
					className="flex flex-col gap-6 w-full border border-border-default border-solid rounded-lg p-6"
				>
					{Boolean(error) && <ErrorAlert error={error} />}

					{mode === "duplicate" && (
						<Alert
							severity="info"
							dismissible
							data-testid="duplication-warning"
						>
							Duplicating a workspace only copies its parameters. No state from
							the old workspace is copied over.
						</Alert>
					)}

					<section className="flex flex-col gap-4">
						<hgroup>
							<h2 className="text-xl font-semibold m-0">General</h2>
							<p className="text-sm text-content-secondary mt-0">
								{permissions.createWorkspaceForAny
									? "Only admins can create workspaces for other users."
									: "The name of your new workspace."}
							</p>
						</hgroup>
						<div>
							{versionId && versionId !== template.active_version_id && (
								<div className="flex flex-col gap-2 pb-4">
									<Label className="text-sm" htmlFor={`${id}-version-id`}>
										Version ID
									</Label>
									<Input id={`${id}-version-id`} value={versionId} disabled />
									<span className="text-xs text-content-secondary">
										This parameter has been preset, and cannot be modified.
									</span>
								</div>
							)}
							<div className="flex gap-4 flex-wrap">
								<div className="flex flex-col gap-2 flex-1">
									<Label className="text-sm" htmlFor={`${id}-workspace-name`}>
										Workspace name
									</Label>
									<div>
										<Input
											id={`${id}-workspace-name`}
											value={form.values.name}
											onChange={(e) => {
												form.setFieldValue("name", e.target.value.trim());
												resetMutation();
											}}
											disabled={creatingWorkspace}
										/>
										<div className="flex gap-2 text-xs text-content-secondary items-center">
											Need a suggestion?
											<Button
												variant="subtle"
												size="sm"
												onClick={async () => {
													await form.setFieldValue("name", suggestedName);
													rerollSuggestedName();
												}}
											>
												{suggestedName}
											</Button>
										</div>
									</div>
								</div>
								{permissions.createWorkspaceForAny && (
									<div className="flex flex-col gap-2 flex-1">
										<Label className="text-sm" htmlFor={`${id}-workspace-name`}>
											Owner
										</Label>
										<UserAutocomplete
											value={owner}
											onChange={(user) => {
												setOwner(user ?? defaultOwner);
											}}
											size="medium"
										/>
									</div>
								)}
							</div>
						</div>
					</section>

					{externalAuth && externalAuth.length > 0 && (
						<section>
							<hgroup>
								<h2 className="text-xl font-semibold mb-0">
									External Authentication
								</h2>
								<p className="text-sm text-content-secondary mt-0">
									This template uses external services for authentication.
								</p>
							</hgroup>
							<div>
								{Boolean(error) && !hasAllRequiredExternalAuth && (
									<Alert severity="error">
										To create a workspace using this template, please connect to
										all required external authentication providers listed below.
									</Alert>
								)}
								{externalAuth.map((auth) => (
									<ExternalAuthButton
										key={auth.id}
										error={error}
										auth={auth}
										isLoading={externalAuthPollingState === "polling"}
										onStartPolling={startPollingExternalAuth}
										displayRetry={externalAuthPollingState === "abandoned"}
									/>
								))}
							</div>
						</section>
					)}

					{parameters.length > 0 && (
						<section className="flex flex-col gap-6">
							<hgroup>
								<h2 className="text-xl font-semibold m-0">Parameters</h2>
								<p className="text-sm text-content-secondary m-0">
									These are the settings used by your template. Immutable
									parameters cannot be modified once the workspace is created.
								</p>
							</hgroup>
							{presets.length > 0 && (
								<Stack direction="column" spacing={2}>
									<div className="flex flex-col gap-2">
										<div className="flex gap-2 items-center">
											<Label className="text-sm">Preset</Label>
											<FeatureStageBadge contentType={"beta"} size="md" />
										</div>
										<div className="flex">
											<SelectFilter
												label="Preset"
												options={presetOptions}
												onSelect={(option) => {
													const index = presetOptions.findIndex(
														(preset) => preset.value === option?.value,
													);
													if (index === -1) {
														return;
													}
													setSelectedPresetIndex(index);
												}}
												placeholder="Select a preset"
												selectedOption={presetOptions[selectedPresetIndex]}
											/>
										</div>
										<span className="flex items-center gap-3">
											<Switch
												id="show-preset-parameters"
												checked={showPresetParameters}
												onCheckedChange={setShowPresetParameters}
											/>
											<Label htmlFor="show-preset-parameters">
												Show preset parameters
											</Label>
										</span>
									</div>
								</Stack>
							)}

							<div className="flex flex-col gap-9">
								{parameters.map((parameter, index) => {
									const parameterField = `rich_parameter_values.${index}`;
									const parameterInputName = `${parameterField}.value`;
									const isPresetParameter = presetParameterNames.includes(
										parameter.name,
									);
									const isDisabled =
										disabledParams?.includes(
											parameter.name.toLowerCase().replace(/ /g, "_"),
										) ||
										(parameter.styling as { disabled?: boolean })?.disabled ||
										creatingWorkspace ||
										isPresetParameter;

									// Hide preset parameters if showPresetParameters is false
									if (!showPresetParameters && isPresetParameter) {
										return null;
									}

									return (
										<DynamicParameter
											{...getFieldHelpers(parameterInputName)}
											key={parameter.name}
											parameter={parameter}
											onChange={(value) =>
												handleChange(parameter, parameterField, value)
											}
											disabled={isDisabled}
											isPreset={isPresetParameter}
										/>
									);
								})}
							</div>
						</section>
					)}

					<div className="flex flex-row justify-end">
						<Button
							type="submit"
							disabled={creatingWorkspace || !hasAllRequiredExternalAuth}
						>
							<Spinner loading={creatingWorkspace} />
							Create workspace
						</Button>
					</div>
				</form>
			</div>
		</>
	);
};
