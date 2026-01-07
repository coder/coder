import type { WorkspaceTransition } from "api/typesGenerated";
import { PlayIcon, SquareIcon, TrashIcon } from "lucide-react";
import type { ComponentProps } from "react";
import { cn } from "utils/cn";

type SVGIcon = typeof PlayIcon;

type SVGIconProps = ComponentProps<SVGIcon>;

const iconByTransition: Record<WorkspaceTransition, SVGIcon> = {
	start: PlayIcon,
	stop: SquareIcon,
	delete: TrashIcon,
};

export const BuildIcon = ({
	transition,
	className,
	...props
}: SVGIconProps & { transition: WorkspaceTransition }) => {
	const Icon = iconByTransition[transition];
	return (
		<Icon
			className={cn(transition === "stop" && "size-icon-xs", className)}
			{...props}
		/>
	);
};
