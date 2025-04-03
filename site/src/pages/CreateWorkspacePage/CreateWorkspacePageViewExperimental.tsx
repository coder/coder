import type { Interpolation, Theme } from "@emotion/react";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { SelectFilter } from "components/Filter/SelectFilter";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Pill } from "components/Pill/Pill";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FormikContextType, useFormik } from "formik";
import { ArrowLeft } from "lucide-react";
import type { WorkspacePermissions } from "modules/permissions/workspaces";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import {
	type FC,
	useCallback,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";
import { Link } from "react-router-dom";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import {
	type AutofillBuildParameter,
	getInitialRichParameterValues,
	useValidationSchemaForRichParameters,
} from "utils/richParameters";
import * as Yup from "yup";
import type {
	CreateWorkspaceMode,
	ExternalAuthPollingState,
} from "./CreateWorkspacePage";
import { ExternalAuthButton } from "./ExternalAuthButton";

export const Language = {
	duplicationWarning:
		"Duplicating a workspace only copies its parameters. No state from the old workspace is copied over.",
} as const;

export interface CreateWorkspacePageViewExperimentalProps {
	mode: CreateWorkspaceMode;
	defaultName?: string | null;
	disabledParams?: string[];
	error: unknown;
	resetMutation: () => void;
	defaultOwner: TypesGen.User;
	template: TypesGen.Template;
	versionId?: string;
	externalAuth: TypesGen.TemplateVersionExternalAuth[];
	externalAuthPollingState: ExternalAuthPollingState;
	startPollingExternalAuth: () => void;
	hasAllRequiredExternalAuth: boolean;
	parameters: TypesGen.TemplateVersionParameter[];
	autofillParameters: AutofillBuildParameter[];
	presets: TypesGen.Preset[];
	permissions: WorkspacePermissions;
	creatingWorkspace: boolean;
	onCancel: () => void;
	onSubmit: (
		req: TypesGen.CreateWorkspaceRequest,
		owner: TypesGen.User,
	) => void;
}

export const CreateWorkspacePageViewExperimental: FC<
	CreateWorkspacePageViewExperimentalProps
> = ({
	mode,
	defaultName,
	disabledParams,
	error,
	resetMutation,
	defaultOwner,
	template,
	versionId,
	externalAuth,
	externalAuthPollingState,
	startPollingExternalAuth,
	hasAllRequiredExternalAuth,
	parameters,
	autofillParameters,
	presets = [],
	permissions,
	creatingWorkspace,
	onSubmit,
	onCancel,
}) => {
	const [owner, setOwner] = useState(defaultOwner);
	const [suggestedName, setSuggestedName] = useState(() =>
		generateWorkspaceName(),
	);
	const id = useId();

	const rerollSuggestedName = useCallback(() => {
		setSuggestedName(() => generateWorkspaceName());
	}, []);

	const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
		useFormik<TypesGen.CreateWorkspaceRequest>({
			initialValues: {
				name: defaultName ?? "",
				template_id: template.id,
				rich_parameter_values: getInitialRichParameterValues(
					parameters,
					autofillParameters,
				),
			},
			validationSchema: Yup.object({
				name: nameValidator("Workspace Name"),
				rich_parameter_values: useValidationSchemaForRichParameters(parameters),
			}),
			enableReinitialize: true,
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
		parameters,
		form.setFieldValue,
	]);

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
							{Language.duplicationWarning}
						</Alert>
					)}

					<section className="flex flex-col gap-4">
						<hgroup>
							<h2 className="text-xl font-semibold m-0">General</h2>
							<p className="text-sm text-content-secondary mt-0">
								{permissions.createWorkspaceForUser
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
								{permissions.createWorkspaceForUser && (
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
									These are the settings used by your template. Please note that
									immutable parameters cannot be modified once the workspace is
									created.
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
									</div>
								</Stack>
							)}

							<div className="flex flex-col gap-9">
								{parameters.map((parameter, index) => {
									const parameterField = `rich_parameter_values.${index}`;
									const parameterInputName = `${parameterField}.value`;
									const isDisabled =
										disabledParams?.includes(
											parameter.name.toLowerCase().replace(/ /g, "_"),
										) ||
										creatingWorkspace ||
										presetParameterNames.includes(parameter.name);

									return (
										<RichParameterInput
											{...getFieldHelpers(parameterInputName)}
											onChange={async (value) => {
												await form.setFieldValue(parameterField, {
													name: parameter.name,
													value,
												});
											}}
											key={parameter.name}
											parameter={parameter}
											parameterAutofill={autofillByName[parameter.name]}
											disabled={isDisabled}
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

const styles = {
	description: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
