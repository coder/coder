import type { FC, ReactNode } from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";

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
				<div className="text-content-primary text-xs">{children}</div>
			</CollapsibleContent>
			<CollapsibleTrigger className="appearance-none bg-transparent border-0 cursor-pointer p-0 text-content-link text-xs hover:underline">
				{expanded ? "Show less" : "Show more"}
			</CollapsibleTrigger>
		</Collapsible>
	);
};
