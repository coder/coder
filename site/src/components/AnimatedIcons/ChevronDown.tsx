import { cn } from "cn";
import { ChevronDownIcon as LucideChevronDown } from "lucide-react";

type ChevronDownIconProps = React.ComponentProps<typeof LucideChevronDown> & {
	/**
	 * Explicitly control rotation state. When omitted, rotation is
	 * driven by the open-state attribute on a parent element
	 * with className="group".
	 */
	open?: boolean;
};

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
				: "group-data-[state=open]:rotate-180 group-data-popup-open:rotate-180",
			className,
		)}
		{...props}
	/>
);
