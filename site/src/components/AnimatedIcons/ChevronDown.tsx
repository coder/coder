import { ChevronDown as LucideChevronDown } from "lucide-react";
import { cn } from "utils/cn";

interface ChevronDownIconProps
	extends React.ComponentProps<typeof LucideChevronDown> {
	/**
	 * Explicitly control rotation state. When omitted, rotation is
	 * driven by Radix's data-state attribute on a parent element
	 * with className="group".
	 */
	open?: boolean;
}

export const ChevronDownIcon: React.FC<ChevronDownIconProps> = ({
	open,
	className,
	...props
}) => (
	<LucideChevronDown
		className={cn(
			"transition-transform",
			open !== undefined
				? open && "rotate-180"
				: "group-data-[state=open]:rotate-180",
			className,
		)}
		{...props}
	/>
);
