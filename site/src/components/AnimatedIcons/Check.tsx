import { CheckIcon as LucideCheckIcon } from "lucide-react";
import { cn } from "utils/cn";

type CheckIconProps = React.ComponentProps<typeof LucideCheckIcon>;

export const CheckIcon: React.FC<CheckIconProps> = ({
	className,
	...props
}) => {
	return (
		<LucideCheckIcon
			className={cn(
				"animate-in fade-in-0 zoom-in-[0.8] duration-300",
				className,
			)}
			{...props}
		/>
	);
};
