import { WorkspaceApp } from "api/typesGenerated";
import { FC } from "react";
import ComputerIcon from "@mui/icons-material/Computer";

export const BaseIcon: FC<{ app: WorkspaceApp }> = ({ app }) => {
  return app.icon ? (
    <img
      alt={`${app.display_name} Icon`}
      src={app.icon}
      style={{
        pointerEvents: "none",
      }}
    />
  ) : (
    <ComputerIcon />
  );
};
