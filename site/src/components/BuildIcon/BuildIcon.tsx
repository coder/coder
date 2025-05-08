import { DeleteOutlined as DeleteOutlined, PlayArrowOutlined as PlayArrowOutlined, StopOutlined as StopOutlined } from "lucide-react";
import type { WorkspaceTransition } from "api/typesGenerated";
import type { ComponentProps } from "react";

type SVGIcon = typeof PlayArrowOutlined;

type SVGIconProps = ComponentProps<SVGIcon>;

const iconByTransition: Record<WorkspaceTransition, SVGIcon> = {
	start: PlayArrowOutlined,
	stop: StopOutlined,
	delete: DeleteOutlined,
};

export const BuildIcon = (
	props: SVGIconProps & { transition: WorkspaceTransition },
) => {
	const Icon = iconByTransition[props.transition];
	return <Icon {...props} />;
};
