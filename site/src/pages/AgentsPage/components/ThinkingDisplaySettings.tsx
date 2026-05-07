import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

type DisplayModeField = "thinking_display_mode" | "code_diff_display_mode";
type DisplayModeValue = UserPreferenceSettings[DisplayModeField];

const options: { value: DisplayModeValue; label: string }[] = [
	{ value: "auto", label: "Auto" },
	{ value: "preview", label: "Preview" },
	{ value: "always_expanded", label: "Always Expanded" },
	{ value: "always_collapsed", label: "Always Collapsed" },
];

const isDisplayModeValue = (value: string): value is DisplayModeValue => {
	return options.some((opt) => opt.value === value);
};

const updateDisplayMode = (
	settings: UserPreferenceSettings,
	field: DisplayModeField,
	value: DisplayModeValue,
): UserPreferenceSettings => {
	switch (field) {
		case "thinking_display_mode":
			return { ...settings, thinking_display_mode: value };
		case "code_diff_display_mode":
			return { ...settings, code_diff_display_mode: value };
		default: {
			const _exhaustive: never = field;
			return _exhaustive;
		}
	}
};

const DisplayModeSettings: FC<{
	field: DisplayModeField;
	title: string;
	description: string;
	ariaLabel: string;
	errorMessage: string;
}> = ({ field, title, description, ariaLabel, errorMessage }) => {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));

	const mode: DisplayModeValue = query.data?.[field] || "auto";

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
						if (!query.data || !isDisplayModeValue(value)) return;
						mutation.mutate(updateDisplayMode(query.data, field, value));
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
			field="thinking_display_mode"
			title="Thinking Display"
			description="How thinking blocks should be displayed by default. 'Auto' fully expands during streaming, then auto-collapses when done. 'Preview' auto-expands with a height constraint during streaming. 'Always Expanded' shows full content. 'Always Collapsed' keeps them collapsed."
			ariaLabel="Thinking display mode"
			errorMessage="Failed to save your thinking display preference."
		/>
	);
};

export const CodeDiffDisplaySettings: FC = () => {
	return (
		<DisplayModeSettings
			field="code_diff_display_mode"
			title="Code Diff Display"
			description="How inline code diff tool calls should be displayed by default."
			ariaLabel="Code diff display mode"
			errorMessage="Failed to save your code diff display preference."
		/>
	);
};
