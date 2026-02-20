import { ChevronDownIcon } from "components/AnimatedIcons/ChevronDown";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import type { FC, ReactNode } from "react";

interface ExpanderProps {
	expanded: boolean;
	setExpanded: (val: boolean) => void;
	children?: ReactNode;
}

export const Expander: FC<ExpanderProps> = ({
	expanded,
	setExpanded,
	children,
}) => {
	return (
		<Collapsible open={expanded} onOpenChange={setExpanded}>
			<CollapsibleContent>
				<p className="flex items-center text-content-secondary text-xs">
					{children}
				</p>
			</CollapsibleContent>
			<CollapsibleTrigger className="cursor-pointer text-content-secondary hover:underline">
				<span className="flex items-center text-xs">
					{expanded ? "Click here to hide" : "Click here to learn more"}
					<ChevronDownIcon open={expanded} className="size-4" />
				</span>
			</CollapsibleTrigger>
		</Collapsible>
	);
};
