import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";

const options: { value: ThinkingDisplayMode; label: string }[] = [
	{ value: "auto", label: "Auto" },
	{ value: "preview", label: "Preview" },
	{ value: "always_expanded", label: "Always Expanded" },
	{ value: "always_collapsed", label: "Always Collapsed" },
];

export const ThinkingDisplaySettings: FC = () => {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));

	const mode: ThinkingDisplayMode = query.data?.thinking_display_mode || "auto";

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Thinking Display
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					How thinking blocks should be displayed by default. 'Auto' fully
					expands during streaming, then auto-collapses when done. 'Preview'
					auto-expands with a height constraint during streaming. 'Always
					Expanded' shows full content. 'Always Collapsed' keeps them collapsed.
				</p>
				<select
					value={mode}
					onChange={(e) =>
						mutation.mutate({
							...query.data,
							thinking_display_mode: e.target.value as ThinkingDisplayMode,
							task_notification_alert_dismissed:
								query.data?.task_notification_alert_dismissed ?? false,
						})
					}
					aria-label="Thinking display mode"
					className="rounded-md border border-border bg-surface-primary px-2 py-1 text-xs text-content-primary"
				>
					{options.map((opt) => (
						<option key={opt.value} value={opt.value}>
							{opt.label}
						</option>
					))}
				</select>
			</div>
		</div>
	);
};
