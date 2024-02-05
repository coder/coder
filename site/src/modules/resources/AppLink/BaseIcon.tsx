import ComputerIcon from "@mui/icons-material/Computer";
import { type FC } from "react";
import type { WorkspaceApp } from "api/typesGenerated";

interface BaseIconProps {
  app: WorkspaceApp;
}

export const BaseIcon: FC<BaseIconProps> = ({ app }) => {
  return app.icon ? (
    <img
      alt={`${app.display_name} Icon`}
      src={app.icon}
      style={{ pointerEvents: "none" }}
    />
  ) : (
    <ComputerIcon />
  );
};
