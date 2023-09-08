import PlayArrowOutlined from "@mui/icons-material/PlayArrowOutlined";
import StopOutlined from "@mui/icons-material/StopOutlined";
import DeleteOutlined from "@mui/icons-material/DeleteOutlined";
import { WorkspaceTransition } from "api/typesGenerated";
import { ComponentProps } from "react";

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
