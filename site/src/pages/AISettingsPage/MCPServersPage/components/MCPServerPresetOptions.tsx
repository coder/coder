import type { FormikContextType } from "formik";
import { useId } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	buildPresetURL,
	builtInMCPPresets,
	githubDefaultAPIBaseURL,
	githubToolsets,
	type MCPPresetOptions,
	slackDefaultAPIBaseURL,
} from "#/modules/mcp/mcpPresets";
import { Field, SwitchField } from "./MCPServerFormFieldPrimitives";
import type { MCPServerFormValues } from "./mcpServerFormLogic";

type MCPServerPresetOptionsProps = {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
	/** The deployment's access URL, which a built-in server must be served from. */
	accessURL?: string;
};

const hostOf = (url: string): string | undefined => {
	try {
		return new URL(url).host;
	} catch {
		return undefined;
	}
};

/**
 * The options of a built-in server. Changing one rewrites the server URL,
 * which stays editable.
 */
export const MCPServerPresetOptions: React.FC<MCPServerPresetOptionsProps> = ({
	form,
	formId,
	disabled,
	accessURL,
}) => {
	const { preset, presetOrigin, presetOptions } = form.values;
	if (preset === "custom") {
		return null;
	}
	const update = (patch: Partial<MCPPresetOptions>) => {
		const next = { ...presetOptions, ...patch };
		void form.setFieldValue("presetOptions", next);
		void form.setFieldValue("url", buildPresetURL(preset, presetOrigin, next));
	};

	const accessHost = accessURL ? hostOf(accessURL) : undefined;
	const urlHost = hostOf(form.values.url);
	const hostDiffers =
		accessHost !== undefined && urlHost !== undefined && urlHost !== accessHost;

	return (
		<div className="flex flex-col gap-4 rounded-lg border border-solid border-border p-4">
			<h3 className="m-0 text-sm font-medium text-content-primary">
				{builtInMCPPresets[preset].title} options
			</h3>
			{hostDiffers && (
				<Alert severity="warning" prominent>
					<AlertDescription>
						The server URL points at {urlHost}, but Coder serves built-in
						servers from {accessHost}. Coder rejects the server unless the URL
						uses the deployment's access URL.
					</AlertDescription>
				</Alert>
			)}
			{preset === "slack" ? (
				<SlackPresetOptions
					formId={formId}
					options={presetOptions}
					disabled={disabled}
					onChange={update}
				/>
			) : (
				<GitHubPresetOptions
					formId={formId}
					options={presetOptions}
					disabled={disabled}
					onChange={update}
				/>
			)}
		</div>
	);
};

type PresetOptionsFieldsProps = {
	formId: string;
	options: MCPPresetOptions;
	disabled: boolean;
	onChange: (patch: Partial<MCPPresetOptions>) => void;
};

const SlackPresetOptions: React.FC<PresetOptionsFieldsProps> = ({
	formId,
	options,
	disabled,
	onChange,
}) => (
	<>
		<Field
			label="Slack API base URL"
			htmlFor={`${formId}-slack-api`}
			description={`Leave as ${slackDefaultAPIBaseURL} unless Slack traffic goes through a proxy.`}
		>
			<Input
				id={`${formId}-slack-api`}
				className="shadow-none"
				value={options.slackAPIBaseURL}
				onChange={(event) => onChange({ slackAPIBaseURL: event.target.value })}
				disabled={disabled}
			/>
		</Field>
		<SwitchField
			label="Only reply in the originating thread"
			checked={options.slackPinThread}
			onCheckedChange={(checked) => onChange({ slackPinThread: checked })}
			disabled={disabled}
		/>
	</>
);

const GitHubPresetOptions: React.FC<PresetOptionsFieldsProps> = ({
	formId,
	options,
	disabled,
	onChange,
}) => (
	<>
		<Field
			label="GitHub API base URL"
			htmlFor={`${formId}-github-api`}
			description={`Use https://HOST/api/v3 for GitHub Enterprise Server. Defaults to ${githubDefaultAPIBaseURL}.`}
		>
			<Input
				id={`${formId}-github-api`}
				className="shadow-none"
				value={options.githubAPIBaseURL}
				onChange={(event) => onChange({ githubAPIBaseURL: event.target.value })}
				disabled={disabled}
			/>
		</Field>
		<fieldset className="m-0 flex min-w-0 flex-col gap-2 border-0 p-0">
			<legend className="mb-1.5 p-0 text-sm font-medium text-content-primary">
				Toolsets
			</legend>
			<div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
				{githubToolsets.map((toolset) => (
					<ToolsetCheckbox
						key={toolset.id}
						label={toolset.label}
						checked={options.githubToolsets.includes(toolset.id)}
						disabled={disabled}
						onCheckedChange={(checked) =>
							onChange({
								githubToolsets: checked
									? [...options.githubToolsets, toolset.id]
									: options.githubToolsets.filter((id) => id !== toolset.id),
							})
						}
					/>
				))}
			</div>
		</fieldset>
		<SwitchField
			label="Read only"
			checked={options.githubReadOnly}
			onCheckedChange={(checked) => onChange({ githubReadOnly: checked })}
			disabled={disabled}
		/>
	</>
);

type ToolsetCheckboxProps = {
	label: string;
	checked: boolean;
	disabled: boolean;
	onCheckedChange: (checked: boolean) => void;
};

const ToolsetCheckbox: React.FC<ToolsetCheckboxProps> = ({
	label,
	checked,
	disabled,
	onCheckedChange,
}) => {
	const id = useId();
	return (
		<div className="flex items-center gap-2">
			<Checkbox
				id={id}
				checked={checked}
				disabled={disabled}
				onCheckedChange={(value) => onCheckedChange(value === true)}
			/>
			<Label htmlFor={id} className="cursor-pointer font-normal">
				{label}
			</Label>
		</div>
	);
};
