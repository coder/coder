import type { WorkspaceTransition } from "api/typesGenerated";
import { PlayIcon, SquareIcon, TrashIcon } from "lucide-react";
import type { ComponentProps } from "react";

type SVGIcon = typeof PlayIcon;

type SVGIconProps = ComponentProps<SVGIcon>;

const iconByTransition: Record<WorkspaceTransition, SVGIcon> = {
	start: PlayIcon,
	stop: SquareIcon,
	delete: TrashIcon,
};

export const BuildIcon = (
	props: SVGIconProps & { transition: WorkspaceTransition },
) => {
	const Icon = iconByTransition[props.transition];
	return <Icon {...props} />;
};
