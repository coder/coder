import type { FC } from "react";
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
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

type DisplayModeOption<T extends string> = { value: T; label: string };

type ThinkingDisplayMode = UserPreferenceSettings["thinking_display_mode"];
type AgentDisplayMode = UserPreferenceSettings["code_diff_display_mode"];

const thinkingDisplayOptions: DisplayModeOption<ThinkingDisplayMode>[] = [
	{ value: "auto", label: "Auto" },
	{ value: "preview", label: "Preview" },
	{ value: "always_expanded", label: "Always expanded" },
	{ value: "always_collapsed", label: "Always collapsed" },
];

const agentDisplayOptions: DisplayModeOption<AgentDisplayMode>[] = [
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
	options: DisplayModeOption<T>[];
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

	const mode = query.data ? getMode(query.data) : defaultValue;

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				{title}
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					{description}
				</p>
				<Select
					value={mode}
					disabled={query.isLoading || !query.data}
					onValueChange={(value: string) => {
						const selected = options.find((opt) => opt.value === value);
						if (!query.data || !selected) return;
						mutation.mutate(updateSettings(selected.value));
					}}
				>
					<SelectTrigger className="w-44 shrink-0" aria-label={ariaLabel}>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{options.map((opt) => (
							<SelectItem key={opt.value} value={opt.value}>
								{opt.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
			{mutation.isError && (
				<p className="m-0 text-xs text-content-destructive">{errorMessage}</p>
			)}
		</div>
	);
};

export const ThinkingDisplaySettings: FC = () => {
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

export const ShellToolDisplaySettings: FC = () => {
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

export const CodeDiffDisplaySettings: FC = () => {
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
