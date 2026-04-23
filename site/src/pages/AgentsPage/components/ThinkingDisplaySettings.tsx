import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

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
				<Select
					value={mode}
					disabled={query.isLoading || !query.data}
					onValueChange={(value: string) => {
						if (!query.data) return;
						mutation.mutate({
							...query.data,
							thinking_display_mode: value as ThinkingDisplayMode,
						});
					}}
				>
					<SelectTrigger
						className="w-44 shrink-0"
						aria-label="Thinking display mode"
					>
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
				<p className="m-0 text-xs text-content-destructive">
					Failed to save your thinking display preference.
				</p>
			)}
		</div>
	);
};
