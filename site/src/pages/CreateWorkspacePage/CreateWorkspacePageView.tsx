import type { Interpolation, Theme } from "@emotion/react";
import FormHelperText from "@mui/material/FormHelperText";
import TextField from "@mui/material/TextField";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { Combobox } from "components/Combobox/Combobox";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Pill } from "components/Pill/Pill";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { Switch } from "components/Switch/Switch";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FormikContextType, useFormik } from "formik";
import type { ExternalAuthPollingState } from "hooks/useExternalAuth";
import { ExternalLinkIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { ClassicParameterFlowDeprecationWarning } from "modules/workspaces/ClassicParameterFlowDeprecationWarning/ClassicParameterFlowDeprecationWarning";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import { type FC, useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
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
import type { CreateWorkspaceMode } from "./CreateWorkspacePage";
import { ExternalAuthButton } from "./ExternalAuthButton";
import type { CreateWorkspacePermissions } from "./permissions";

export const Language = {
	duplicationWarning:
		"Duplicating a workspace only copies its parameters. No state from the old workspace is copied over.",
} as const;

interface CreateWorkspacePageViewProps {
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
	permissions: CreateWorkspacePermissions;
	templatePermissions: { canUpdateTemplate: boolean };
	creatingWorkspace: boolean;
	canUpdateTemplate?: boolean;
	onCancel: () => void;
	onSubmit: (
		req: TypesGen.CreateWorkspaceRequest,
		owner: TypesGen.User,
	) => void;
}

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = ({
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
	templatePermissions,
	creatingWorkspace,
	canUpdateTemplate,
	onSubmit,
	onCancel,
}) => {
	const getLink = useLinks();
	const [owner, setOwner] = useState(defaultOwner);
	const [suggestedName, setSuggestedName] = useState(() =>
		generateWorkspaceName(),
	);
	const [showPresetParameters, setShowPresetParameters] = useState(false);

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
		<Margins size="medium">
			<PageHeader
				actions={
					<Stack direction="row" spacing={2}>
						{canUpdateTemplate && (
							<Button asChild size="sm" variant="outline">
								<Link
									to={`/templates/${template.organization_name}/${template.name}/versions/${versionId}/edit`}
								>
									<ExternalLinkIcon />
									View source
								</Link>
							</Button>
						)}
						<Button size="sm" variant="outline" onClick={onCancel}>
							Cancel
						</Button>
					</Stack>
				}
			>
				<Stack direction="row">
					<Avatar
						variant="icon"
						size="lg"
						src={template.icon}
						fallback={template.name}
					/>

					<div>
						<PageHeaderTitle>
							{template.display_name.length > 0
								? template.display_name
								: template.name}
						</PageHeaderTitle>

						<PageHeaderSubtitle condensed>New workspace</PageHeaderSubtitle>
					</div>

					{template.deprecated && <Pill type="warning">Deprecated</Pill>}
				</Stack>
			</PageHeader>

			<ClassicParameterFlowDeprecationWarning
				templateSettingsLink={`${getLink(
					linkToTemplate(template.organization_name, template.name),
				)}/settings`}
				isEnabled={templatePermissions.canUpdateTemplate}
			/>

			<HorizontalForm
				name="create-workspace-form"
				onSubmit={form.handleSubmit}
				css={{ padding: "16px 0" }}
			>
				{Boolean(error) && <ErrorAlert error={error} />}

				{mode === "duplicate" && (
					<Alert severity="info" dismissible data-testid="duplication-warning">
						{Language.duplicationWarning}
					</Alert>
				)}

				{/* General info */}
				<FormSection
					title="General"
					description={
						permissions.createWorkspaceForAny
							? "The name of the workspace and its owner. Only admins can create workspaces for other users."
							: "The name of your new workspace."
					}
				>
					<FormFields>
						{versionId && versionId !== template.active_version_id && (
							<Stack spacing={1} css={styles.hasDescription}>
								<TextField
									disabled
									fullWidth
									value={versionId}
									label="Version ID"
								/>
								<span css={styles.description}>
									This parameter has been preset, and cannot be modified.
								</span>
							</Stack>
						)}

						<div>
							<TextField
								{...getFieldHelpers("name")}
								disabled={creatingWorkspace}
								// resetMutation facilitates the clearing of validation errors
								onChange={onChangeTrimmed(form, resetMutation)}
								fullWidth
								label="Workspace Name"
							/>
							<FormHelperText data-chromatic="ignore">
								Need a suggestion?{" "}
								<Button
									variant="subtle"
									size="sm"
									css={styles.nameSuggestion}
									onClick={async () => {
										await form.setFieldValue("name", suggestedName);
										rerollSuggestedName();
									}}
								>
									{suggestedName}
								</Button>
							</FormHelperText>
						</div>

						{permissions.createWorkspaceForAny && (
							<UserAutocomplete
								value={owner}
								onChange={(user) => {
									setOwner(user ?? defaultOwner);
								}}
								label="Owner"
								size="medium"
							/>
						)}
					</FormFields>
				</FormSection>

				{externalAuth && externalAuth.length > 0 && (
					<FormSection
						title="External Authentication"
						description="This template uses external services for authentication."
					>
						<FormFields>
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
						</FormFields>
					</FormSection>
				)}

				{parameters.length > 0 && (
					<FormSection
						title="Parameters"
						description="These are the settings used by your template. Please note that immutable parameters cannot be modified once the workspace is created."
					>
						{/* The parameter fields are densely packed and carry significant information,
                hence they require additional vertical spacing for better readability and
                user experience. */}
						<FormFields css={{ gap: 36 }}>
							{presets.length > 0 && (
								<Stack direction="column" spacing={2}>
									<Stack direction="row" spacing={2} alignItems="center">
										<span css={styles.description}>
											Select a preset to get started
										</span>
									</Stack>
									<Stack direction="column" spacing={2}>
										<Stack direction="row" spacing={2}>
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
										</Stack>
										{/* Only show the preset parameter visibility toggle if preset parameters are actually being modified, otherwise it has no effect. */}
										{presetParameterNames.length > 0 && (
											<div
												css={{
													display: "flex",
													alignItems: "center",
													gap: "8px",
												}}
											>
												<Switch
													id="show-preset-parameters"
													checked={showPresetParameters}
													onCheckedChange={setShowPresetParameters}
												/>
												<label
													htmlFor="show-preset-parameters"
													css={styles.description}
												>
													Show preset parameters
												</label>
											</div>
										)}
									</Stack>
								</Stack>
							)}

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
									creatingWorkspace ||
									isPresetParameter;

								// Hide preset parameters if showPresetParameters is false
								if (!showPresetParameters && isPresetParameter) {
									return null;
								}

								return (
									<div key={parameter.name}>
										<RichParameterInput
											{...getFieldHelpers(parameterInputName)}
											onChange={async (value) => {
												await form.setFieldValue(parameterField, {
													name: parameter.name,
													value,
												});
											}}
											parameter={parameter}
											parameterAutofill={autofillByName[parameter.name]}
											disabled={isDisabled}
											isPreset={isPresetParameter}
										/>
									</div>
								);
							})}
						</FormFields>
					</FormSection>
				)}

				<FormFooter>
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button
						type="submit"
						disabled={creatingWorkspace || !hasAllRequiredExternalAuth}
					>
						<Spinner loading={creatingWorkspace} />
						Create workspace
					</Button>
				</FormFooter>
			</HorizontalForm>
		</Margins>
	);
};

const styles = {
	nameSuggestion: (theme) => ({
		color: theme.roles.notice.fill.solid,
		padding: "4px 8px",
		lineHeight: "inherit",
		fontSize: "inherit",
		height: "unset",
		minWidth: "unset",
	}),
	hasDescription: {
		paddingBottom: 16,
	},
	description: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
