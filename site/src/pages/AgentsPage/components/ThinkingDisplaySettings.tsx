import type { FC } from "react";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import { useThinkingDisplayMode } from "../hooks/useThinkingDisplayMode";

const options: { value: ThinkingDisplayMode; label: string }[] = [
	{ value: "auto", label: "Auto" },
	{ value: "preview", label: "Preview" },
	{ value: "always_expanded", label: "Always Expanded" },
	{ value: "always_collapsed", label: "Always Collapsed" },
];

export const ThinkingDisplaySettings: FC = () => {
	const { mode, setMode } = useThinkingDisplayMode();

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
					onChange={(e) => setMode(e.target.value as ThinkingDisplayMode)}
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
