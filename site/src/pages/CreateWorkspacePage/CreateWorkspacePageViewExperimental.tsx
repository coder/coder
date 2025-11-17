import type * as TypesGen from "api/typesGenerated";
import type { FriendlyDiagnostic, PreviewParameter } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Combobox } from "components/Combobox/Combobox";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FormikContextType, useFormik } from "formik";
import type { ExternalAuthPollingState } from "hooks/useExternalAuth";
import { ArrowLeft, CircleHelp, ExternalLinkIcon } from "lucide-react";
import { useSyncFormParameters } from "modules/hooks/useSyncFormParameters";
import {
	Diagnostics,
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
	useRef,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import { docs } from "utils/docs";
import { nameValidator } from "utils/formUtils";
import type { AutofillBuildParameter } from "utils/richParameters";
import * as Yup from "yup";
import type { CreateWorkspaceMode } from "./CreateWorkspacePage";
import { ExternalAuthButton } from "./ExternalAuthButton";
import type { CreateWorkspacePermissions } from "./permissions";

interface CreateWorkspacePageViewExperimentalProps {
	autofillParameters: AutofillBuildParameter[];
	canUpdateTemplate?: boolean;
	creatingWorkspace: boolean;
	defaultName?: string | null;
	defaultOwner: TypesGen.User;
	diagnostics: readonly FriendlyDiagnostic[];
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
	sendMessage: (message: Record<string, string>, ownerId?: string) => void;
	startPollingExternalAuth: () => void;
	owner: TypesGen.User;
	setOwner: (user: TypesGen.User) => void;
}

export const CreateWorkspacePageViewExperimental: FC<
	CreateWorkspacePageViewExperimentalProps
> = ({
	autofillParameters,
	canUpdateTemplate,
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
	owner,
	setOwner,
}) => {
	const [suggestedName, setSuggestedName] = useState(generateWorkspaceName);
	const [showPresetParameters, setShowPresetParameters] = useState(false);
	const id = useId();
	const workspaceNameInputRef = useRef<HTMLInputElement>(null);
	const rerollSuggestedName = useCallback(() => {
		setSuggestedName(() => generateWorkspaceName());
	}, []);

	const autofillByName = Object.fromEntries(
		autofillParameters.map((param) => [param.name, param]),
	);

	// Only touched fields are sent to the websocket
	// Autofilled parameters are marked as touched since they have been modified
	const initialTouched = Object.fromEntries(
		parameters.filter((p) => autofillByName[p.name]).map((p) => [p.name, true]),
	);

	// The form parameters values hold the working state of the parameters that will be submitted when creating a workspace
	// 1. The form parameter values are initialized from the websocket response when the form is mounted
	// 2. Only touched form fields are sent to the websocket, a field is touched if edited by the user or set by autofill
	// 3. The websocket response may add or remove parameters, these are added or removed from the form values in the useSyncFormParameters hook
	// 4. All existing form parameters are updated to match the websocket response in the useSyncFormParameters hook
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
			initialTouched,
			validationSchema: Yup.object({
				name: nameValidator("Workspace Name"),
				rich_parameter_values:
					useValidationSchemaForDynamicParameters(parameters),
			}),
			enableReinitialize: false,
			validateOnChange: true,
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

	useEffect(() => {
		if (form.submitCount > 0 && Object.keys(form.errors).length > 0) {
			workspaceNameInputRef.current?.scrollIntoView({
				behavior: "smooth",
				block: "center",
			});
			workspaceNameInputRef.current?.focus();
		}
	}, [form.submitCount, form.errors]);

	const [presetOptions, setPresetOptions] = useState([
		{ displayName: "None", value: "undefined", icon: "", description: "" },
	]);
	const [selectedPresetIndex, setSelectedPresetIndex] = useState(0);
	// Build options and keep default label/value in sync
	useEffect(() => {
		const options = [
			{ displayName: "None", value: "undefined", icon: "", description: "" },
			...presets.map((preset) => ({
				displayName: preset.Default ? `${preset.Name} (Default)` : preset.Name,
				value: preset.ID,
				icon: preset.Icon,
				description: preset.Description,
			})),
		];
		setPresetOptions(options);
		const defaultPreset = presets.find((p) => p.Default);
		if (defaultPreset) {
			const idx = presets.indexOf(defaultPreset) + 1; // +1 for "None"
			setSelectedPresetIndex(idx);
			form.setFieldValue("template_version_preset_id", defaultPreset.ID);
		} else {
			setSelectedPresetIndex(0); // Explicitly set to "None"
			form.setFieldValue("template_version_preset_id", undefined);
		}
	}, [presets, form.setFieldValue]);

	const [presetParameterNames, setPresetParameterNames] = useState<string[]>(
		[],
	);

	// include any modified parameters and all touched parameters to the websocket request
	const sendDynamicParamsRequest = useCallback(
		(
			parameters: Array<{ parameter: PreviewParameter; value: string }>,
			ownerId?: string,
		) => {
			const formInputs: Record<string, string> = {};
			const formParameters = form.values.rich_parameter_values ?? [];

			for (const { parameter, value } of parameters) {
				formInputs[parameter.name] = value;
			}

			for (const [fieldName, isTouched] of Object.entries(form.touched)) {
				if (
					isTouched &&
					!parameters.some((p) => p.parameter.name === fieldName)
				) {
					const param = formParameters.find((p) => p.name === fieldName);
					if (param?.value) {
						formInputs[fieldName] = param.value;
					}
				}
			}

			sendMessage(formInputs, ownerId);
		},
		[form.touched, form.values.rich_parameter_values, sendMessage],
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

		const currentValues = form.values.rich_parameter_values ?? [];

		const updates: Array<{
			field: string;
			fieldValue: TypesGen.WorkspaceBuildParameter;
			parameter: PreviewParameter;
			presetValue: string;
		}> = [];

		for (const presetParameter of selectedPreset.Parameters) {
			const parameterIndex = parameters.findIndex(
				(p) => p.name === presetParameter.Name,
			);
			if (parameterIndex === -1) continue;

			const parameterField = `rich_parameter_values.${parameterIndex}`;
			const parameter = parameters[parameterIndex];
			const currentValue = currentValues.find(
				(p) => p.name === presetParameter.Name,
			)?.value;

			if (currentValue !== presetParameter.Value) {
				updates.push({
					field: parameterField,
					fieldValue: {
						name: presetParameter.Name,
						value: presetParameter.Value,
					},
					parameter,
					presetValue: presetParameter.Value,
				});
			}
		}

		if (updates.length > 0) {
			for (const update of updates) {
				form.setFieldValue(update.field, update.fieldValue);
				form.setFieldTouched(update.parameter.name, true);
			}

			sendDynamicParamsRequest(
				updates.map((update) => ({
					parameter: update.parameter,
					value: update.presetValue,
				})),
			);
		}
	}, [
		presetOptions,
		selectedPresetIndex,
		presets,
		form.setFieldValue,
		form.setFieldTouched,
		parameters,
		form.values.rich_parameter_values,
		sendDynamicParamsRequest,
	]);

	const handleOwnerChange = (user: TypesGen.User) => {
		setOwner(user);
		sendDynamicParamsRequest([], user.id);
	};

	const handleChange = async (
		parameter: PreviewParameter,
		parameterField: string,
		value: string,
	) => {
		const currentFormValue = form.values.rich_parameter_values?.find(
			(p) => p.name === parameter.name,
		)?.value;

		await form.setFieldValue(parameterField, {
			name: parameter.name,
			value,
		});

		// Only send the request if the value has changed from the form value
		if (currentFormValue !== value) {
			form.setFieldTouched(parameter.name, true);
			sendDynamicParamsRequest([{ parameter, value }]);
		}
	};

	useSyncFormParameters({
		parameters,
		formValues: form.values.rich_parameter_values ?? [],
		setFieldValue: form.setFieldValue,
	});

	return (
		<>
			<div className="sticky top-5 ml-10">
				<button
					onClick={onCancel}
					type="button"
					className="flex items-center gap-2 bg-transparent border-none text-content-secondary hover:text-content-primary translate-y-12"
				>
					<ArrowLeft size={20} />
					Go back
				</button>
			</div>
			<div className="flex flex-col gap-6 max-w-screen-md mx-auto">
				<header className="flex flex-col items-start gap-3 mt-10">
					<div className="flex items-center gap-2 justify-between w-full">
						<span className="flex items-center gap-2">
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
							{template.deprecated && (
								<Badge variant="warning" size="sm">
									Deprecated
								</Badge>
							)}
						</span>
						{canUpdateTemplate && (
							<Button asChild size="sm" variant="outline">
								<RouterLink
									to={`/templates/${template.organization_name}/${template.name}/versions/${versionId}/edit`}
								>
									<ExternalLinkIcon />
									View source
								</RouterLink>
							</Button>
						)}
					</div>
					<span className="flex flex-row items-center gap-2">
						<h1 className="text-3xl font-semibold m-0">New workspace</h1>

						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<CircleHelp className="size-icon-xs text-content-secondary" />
								</TooltipTrigger>
								<TooltipContent className="max-w-xs text-sm">
									Dynamic Parameters enhances Coder's existing parameter system
									with real-time validation, conditional parameter behavior, and
									richer input types.
									<br />
									<Link
										href={docs(
											"/admin/templates/extending-templates/dynamic-parameters",
										)}
									>
										View docs
									</Link>
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</span>
				</header>

				<form
					onSubmit={form.handleSubmit}
					aria-label="Create workspace form"
					className="flex flex-col gap-10 w-full border border-border-default border-solid rounded-lg p-6"
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
									<div className="flex flex-col">
										<Input
											id={`${id}-workspace-name`}
											ref={workspaceNameInputRef}
											value={form.values.name}
											onChange={(e) => {
												form.setFieldValue("name", e.target.value.trim());
												resetMutation();
											}}
											disabled={creatingWorkspace}
										/>
										{form.touched.name && form.errors.name && (
											<div className="text-content-destructive text-xs mt-2">
												{form.errors.name}
											</div>
										)}
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
												handleOwnerChange(user ?? defaultOwner);
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
								<h2 className="text-xl font-semibold m-0">
									External Authentication
								</h2>
								<p className="text-sm text-content-secondary mt-0">
									This template uses external services for authentication.
								</p>
							</hgroup>
							<div className="flex flex-col gap-4">
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

					{parameters.length === 0 && diagnostics.length > 0 && (
						<Diagnostics diagnostics={diagnostics} />
					)}

					{parameters.length > 0 && (
						<section className="flex flex-col gap-9">
							<hgroup>
								<h2 className="text-xl font-semibold m-0">Parameters</h2>
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
							{diagnostics.length > 0 && (
								<Diagnostics diagnostics={diagnostics} />
							)}
							{presets.length > 0 && (
								<div className="flex flex-col gap-2">
									<div className="flex gap-2 items-center">
										<Label className="text-sm">Preset</Label>
									</div>
									<div className="flex flex-col gap-4">
										<div className="max-w-lg">
											<Combobox
												value={
													presetOptions[selectedPresetIndex]?.displayName || ""
												}
												options={presetOptions}
												placeholder="Select a preset"
												onSelect={(value) => {
													const index = presetOptions.findIndex(
														(preset) => preset.value === value,
													);
													if (index === -1) {
														return;
													}
													setSelectedPresetIndex(index);
													form.setFieldValue(
														"template_version_preset_id",
														// "undefined" string is equivalent to using None option
														// Combobox requires a value in order to correctly highlight the None option
														presetOptions[index].value === "undefined"
															? undefined
															: presetOptions[index].value,
													);
												}}
											/>
										</div>
										{/* Only show the preset parameter visibility toggle if preset parameters are actually being modified, otherwise it is ineffectual */}
										{presetParameterNames.length > 0 && (
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
										)}
									</div>
								</div>
							)}

							<div className="flex flex-col gap-9">
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
									const isPresetParameter = presetParameterNames.includes(
										parameter.name,
									);
									const isDisabled =
										disabledParams?.includes(
											parameter.name.toLowerCase().replace(/ /g, "_"),
										) ||
										parameter.styling?.disabled ||
										creatingWorkspace ||
										isPresetParameter;

									// Always show preset parameters if they have any diagnostics
									if (
										!showPresetParameters &&
										isPresetParameter &&
										parameter.diagnostics.length === 0
									) {
										return null;
									}

									return (
										<DynamicParameter
											key={parameter.name}
											parameter={parameter}
											onChange={(value) =>
												handleChange(parameter, parameterField, value)
											}
											disabled={isDisabled}
											isPreset={isPresetParameter}
											autofill={autofillByName[parameter.name] !== undefined}
											value={formValue}
										/>
									);
								})}
							</div>
						</section>
					)}

					<div className="flex flex-row justify-end">
						<Button
							type="submit"
							disabled={
								creatingWorkspace ||
								!hasAllRequiredExternalAuth ||
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
							<Spinner loading={creatingWorkspace} />
							Create workspace
						</Button>
					</div>
				</form>
			</div>
		</>
	);
};
