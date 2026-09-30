import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type AutomationLabelProps = {
	automationId: string;
	inputId?: string;
	automationName?: string;
};

export const AutomationLabel: React.FC<AutomationLabelProps> = ({
	automationId,
	inputId,
	automationName,
}) => {
	const inputPart = inputId ? ` · input ${inputId.slice(0, 8)}` : "";
	const nameOrId = automationName ?? automationId;
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge
					asChild
					size="sm"
					variant="outline"
					className="min-w-0 max-w-full gap-0"
				>
					{/* The aria-label keeps the separator spaces that the split
					    spans would drop from the accessible name. */}
					<button
						type="button"
						className="cursor-default"
						aria-label={`Automation run · ${nameOrId}${inputPart}`}
					>
						<span className="shrink-0 whitespace-pre">Automation run · </span>
						<span className="min-w-0 truncate">{nameOrId}</span>
						{inputPart && (
							<span className="shrink-0 whitespace-pre">{inputPart}</span>
						)}
					</button>
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs break-words">
				{automationName ? (
					<div>Automation: {automationName}</div>
				) : (
					<div>Automation name unavailable.</div>
				)}
				<div className="font-mono">Automation ID: {automationId}</div>
				{inputId && <div className="font-mono">Input ID: {inputId}</div>}
			</TooltipContent>
		</Tooltip>
	);
};
