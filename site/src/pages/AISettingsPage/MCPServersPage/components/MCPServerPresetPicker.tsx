import { cn } from "cn";
import { PlugIcon } from "lucide-react";
import { RadioGroup as RadioGroupPrimitive } from "radix-ui";
import { useId } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import {
	type BuiltInMCPServerId,
	builtInMCPPresets,
	type MCPServerPresetId,
} from "#/modules/mcp/mcpPresets";

type MCPServerPresetPickerProps = {
	value: MCPServerPresetId;
	onChange: (preset: MCPServerPresetId) => void;
	disabled: boolean;
};

const builtIns: readonly BuiltInMCPServerId[] = ["slack", "github"];

/** Cards for choosing what to add: a built-in server or a custom endpoint. */
export const MCPServerPresetPicker: React.FC<MCPServerPresetPickerProps> = ({
	value,
	onChange,
	disabled,
}) => {
	const headingId = useId();
	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<div className="flex flex-col gap-0.5">
				<h2
					id={headingId}
					className="m-0 text-sm font-medium text-content-primary"
				>
					Start from
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Built-in servers fill in the details for you, and you can still edit
					them. Each user adds their own token.
				</p>
			</div>
			<RadioGroup
				value={value}
				onValueChange={(next) => onChange(next as MCPServerPresetId)}
				disabled={disabled}
				aria-labelledby={headingId}
				className="gap-3 sm:grid-cols-3"
			>
				{builtIns.map((id) => (
					<PresetCard
						key={id}
						value={id}
						title={builtInMCPPresets[id].title}
						summary={builtInMCPPresets[id].summary}
						icon={
							<ExternalImage
								src={builtInMCPPresets[id].iconURL}
								alt=""
								className="size-6 object-contain"
							/>
						}
					/>
				))}
				<PresetCard
					value="custom"
					title="Custom endpoint"
					summary="Connect any MCP server by URL."
					icon={<PlugIcon aria-hidden className="size-6" />}
				/>
			</RadioGroup>
		</section>
	);
};

type PresetCardProps = {
	value: MCPServerPresetId;
	title: string;
	summary: string;
	icon: React.ReactNode;
};

const PresetCard: React.FC<PresetCardProps> = ({
	value,
	title,
	summary,
	icon,
}) => (
	<RadioGroupPrimitive.Item
		value={value}
		className={cn(
			"flex cursor-pointer items-start gap-3 rounded-lg border border-solid bg-surface-primary p-4 text-left transition-colors hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed border-border data-[state=checked]:border-content-link disabled:opacity-50",
		)}
	>
		<span className="flex size-6 shrink-0 items-center justify-center text-content-secondary">
			{icon}
		</span>
		<span className="flex min-w-0 flex-col gap-0.5">
			<span className="text-sm font-medium text-content-primary">{title}</span>
			<span className="text-xs text-content-secondary">{summary}</span>
		</span>
	</RadioGroupPrimitive.Item>
);
