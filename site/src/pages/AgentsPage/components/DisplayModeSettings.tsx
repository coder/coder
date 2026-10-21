import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type {
	UpdateUserPreferenceSettingsRequest,
	UserPreferenceSettings,
} from "#/api/typesGenerated";
import {
	type SettingsSelectOption,
	SettingsSelectRow,
} from "./SettingsSelectRow";

type ThinkingDisplayMode = UserPreferenceSettings["thinking_display_mode"];
type AgentDisplayMode = UserPreferenceSettings["code_diff_display_mode"];

const thinkingDisplayOptions: SettingsSelectOption<ThinkingDisplayMode>[] = [
	{ value: "auto", label: "Auto" },
	{ value: "preview", label: "Preview" },
	{ value: "always_expanded", label: "Always expanded" },
	{ value: "always_collapsed", label: "Always collapsed" },
];

const agentDisplayOptions: SettingsSelectOption<AgentDisplayMode>[] = [
	{ value: "auto", label: "Auto" },
	{ value: "always_expanded", label: "Always expanded" },
	{ value: "always_collapsed", label: "Always collapsed" },
];

type DisplayModeSettingsProps<T extends string> = {
	title: string;
	description: string;
	ariaLabel: string;
	errorMessage: string;
	defaultValue: T;
	options: SettingsSelectOption<T>[];
	getMode: (settings: UserPreferenceSettings) => T;
	updateSettings: (value: T) => UpdateUserPreferenceSettingsRequest;
};

const DisplayModeSettings = <T extends string>({
	title,
	description,
	ariaLabel,
	errorMessage,
	defaultValue,
	options,
	getMode,
	updateSettings,
}: DisplayModeSettingsProps<T>) => {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));

	return (
		<SettingsSelectRow
			title={title}
			description={description}
			ariaLabel={ariaLabel}
			value={query.data ? getMode(query.data) : defaultValue}
			disabled={query.isLoading || !query.data}
			options={options}
			onValueChange={(value) => {
				if (!query.data) return;
				mutation.mutate(updateSettings(value));
			}}
			errorMessage={mutation.isError ? errorMessage : undefined}
		/>
	);
};

export const ThinkingDisplaySettings: React.FC = () => {
	return (
		<DisplayModeSettings
			title="Thinking display"
			description="How thinking blocks should be displayed by default. 'Auto' fully expands during streaming, then auto-collapses when done. 'Preview' auto-expands with a height constraint during streaming. 'Always expanded' shows full content. 'Always collapsed' keeps them collapsed."
			ariaLabel="Thinking display mode"
			errorMessage="Failed to save your thinking display preference."
			defaultValue="auto"
			options={thinkingDisplayOptions}
			getMode={(settings) => settings.thinking_display_mode}
			updateSettings={(value) => ({
				thinking_display_mode: value,
			})}
		/>
	);
};

export const ShellToolDisplaySettings: React.FC = () => {
	return (
		<DisplayModeSettings
			title="Shell output display"
			description="How shell command output should be displayed by default. 'Auto' opens running commands and completed commands with output, then keeps empty output collapsed. 'Always expanded' opens shell output by default. 'Always collapsed' keeps it collapsed."
			ariaLabel="Shell output display mode"
			errorMessage="Failed to save your shell output display preference."
			defaultValue="auto"
			options={agentDisplayOptions}
			getMode={(settings) => settings.shell_tool_display_mode}
			updateSettings={(value) => ({
				shell_tool_display_mode: value,
			})}
		/>
	);
};

export const CodeDiffDisplaySettings: React.FC = () => {
	return (
		<DisplayModeSettings
			title="Code diff display"
			description="Controls how code edit diffs appear. 'Auto' starts single-file writes collapsed and opens multi-file edits with a height-constrained preview. 'Always expanded' opens diffs by default; 'Always collapsed' keeps them collapsed."
			ariaLabel="Code diff display mode"
			errorMessage="Failed to save your code diff display preference."
			defaultValue="auto"
			options={agentDisplayOptions}
			getMode={(settings) => settings.code_diff_display_mode}
			updateSettings={(value) => ({
				code_diff_display_mode: value,
			})}
		/>
	);
};
