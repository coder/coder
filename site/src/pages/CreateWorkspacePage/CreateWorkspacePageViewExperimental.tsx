import type * as TypesGen from "api/typesGenerated";
import type { PreviewDiagnostics, PreviewParameter } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Pill } from "components/Pill/Pill";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FormikContextType, useFormik } from "formik";
import { useDebouncedFunction } from "hooks/debounce";
import { ArrowLeft, CircleAlert, TriangleAlert } from "lucide-react";
import {
	DynamicParameter,
	getInitialParameterValues,
	useValidationSchemaForDynamicParameters,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import {
	type FC,
	useCallback,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { nameValidator } from "utils/formUtils";
import type { AutofillBuildParameter } from "utils/richParameters";
import * as Yup from "yup";
import type {
	CreateWorkspaceMode,
	ExternalAuthPollingState,
} from "./CreateWorkspacePage";
import { ExperimentalFormContext } from "./ExperimentalFormContext";
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
	owner: TypesGen.User;
	setOwner: (user: TypesGen.User) => void;
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
	owner,
	setOwner,
}) => {
	const experimentalFormContext = useContext(ExperimentalFormContext);
	const [suggestedName, setSuggestedName] = useState(() =>
		generateWorkspaceName(),
	);
	const [showPresetParameters, setShowPresetParameters] = useState(false);
	const id = useId();
	const workspaceNameInputRef = useRef<HTMLInputElement>(null);
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
		if (form.submitCount > 0 && form.errors) {
			workspaceNameInputRef.current?.scrollIntoView({
				behavior: "smooth",
				block: "center",
			});
			workspaceNameInputRef.current?.focus();
		}
	}, [form.submitCount, form.errors]);

	const [presetOptions, setPresetOptions] = useState([
		{ label: "None", value: "None" },
	]);
	useEffect(() => {
		setPresetOptions([
			{ label: "None", value: "None" },
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

	// send the last user modified parameter and all touched parameters to the websocket
	const sendDynamicParamsRequest = (
		parameter: PreviewParameter,
		value: string,
	) => {
		const formInputs: { [k: string]: string } = {};
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
			form.setFieldTouched(parameter.name, true);
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
			form.setFieldTouched(parameter.name, true);
			sendDynamicParamsRequest(parameter, value);
		}
	};

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
				<header className="flex flex-col items-start gap-2 mt-10">
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

					{experimentalFormContext && (
						<Button
							size="sm"
							variant="subtle"
							onClick={experimentalFormContext.toggleOptedOut}
						>
							Go back to the classic workspace creation flow
						</Button>
					)}
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

					{parameters.length > 0 && (
						<section className="flex flex-col gap-9">
							<hgroup>
								<h2 className="text-xl font-semibold m-0">Parameters</h2>
								<p className="text-sm text-content-secondary m-0">
									These are the settings used by your template. Immutable
									parameters cannot be modified once the workspace is created.
								</p>
							</hgroup>
							{diagnostics.length > 0 && (
								<Diagnostics diagnostics={diagnostics} />
							)}
							{presets.length > 0 && (
								<div className="flex flex-col gap-2">
									<div className="flex gap-2 items-center">
										<Label className="text-sm">Preset</Label>
										<FeatureStageBadge contentType={"beta"} size="md" />
									</div>
									<div className="flex flex-col gap-4">
										<div className="max-w-lg">
											<Select
												onValueChange={(option) => {
													const index = presetOptions.findIndex(
														(preset) => preset.value === option,
													);
													if (index === -1) {
														return;
													}
													setSelectedPresetIndex(index);
												}}
											>
												<SelectTrigger>
													<SelectValue placeholder={"Select a preset"} />
												</SelectTrigger>
												<SelectContent>
													{presetOptions.map((option) => (
														<SelectItem key={option.value} value={option.value}>
															{option.label}
														</SelectItem>
													))}
												</SelectContent>
											</Select>
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
								</div>
							)}

							<div className="flex flex-col gap-9">
								{parameters.map((parameter, index) => {
									const parameterField = `rich_parameter_values.${index}`;
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

									// Hide preset parameters if showPresetParameters is false
									if (!showPresetParameters && isPresetParameter) {
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

interface DiagnosticsProps {
	diagnostics: PreviewParameter["diagnostics"];
}

const Diagnostics: FC<DiagnosticsProps> = ({ diagnostics }) => {
	return (
		<div className="flex flex-col gap-4">
			{diagnostics.map((diagnostic, index) => (
				<div
					key={`diagnostic-${diagnostic.summary}-${index}`}
					className={`text-xs font-semibold flex flex-col rounded-md border px-3.5 py-3.5 border-solid
                        ${
													diagnostic.severity === "error"
														? "text-content-primary border-border-destructive bg-content-destructive/15"
														: "text-content-primary border-border-warning bg-content-warning/15"
												}`}
				>
					<div className="flex flex-row items-start">
						{diagnostic.severity === "error" && (
							<CircleAlert
								className="me-2 inline-flex shrink-0 text-content-destructive size-icon-sm"
								aria-hidden="true"
							/>
						)}
						{diagnostic.severity === "warning" && (
							<TriangleAlert
								className="me-2 inline-flex shrink-0 text-content-warning size-icon-sm"
								aria-hidden="true"
							/>
						)}
						<div className="flex flex-col gap-3">
							<p className="m-0">{diagnostic.summary}</p>
							{diagnostic.detail && <p className="m-0">{diagnostic.detail}</p>}
						</div>
					</div>
				</div>
			))}
		</div>
	);
};
