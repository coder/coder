import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type AutomationLabelProps = {
	automationId: string;
	inputId?: string;
	// Undefined when the automation was deleted or is not readable by the
	// viewer; the label then names the automation by its ID.
	automationName?: string;
};

export const AutomationLabel: React.FC<AutomationLabelProps> = ({
	automationId,
	inputId,
	automationName,
}) => {
	const inputPart = inputId ? ` · input ${inputId.slice(0, 8)}` : "";
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge asChild size="sm" variant="outline" className="max-w-full">
					<button type="button" className="cursor-default">
						<span className="truncate">
							{`Automation run · ${automationName ?? automationId}${inputPart}`}
						</span>
					</button>
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="top" className="font-mono">
				<div>Automation ID: {automationId}</div>
				{inputId && <div>Input ID: {inputId}</div>}
			</TooltipContent>
		</Tooltip>
	);
};
