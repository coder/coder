import ErrorOutline from "@mui/icons-material/ErrorOutline";
import Tooltip, {
  type TooltipProps,
  tooltipClasses,
} from "@mui/material/Tooltip";
import type { FC, ReactNode } from "react";
import type { Workspace } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Pill } from "components/Pill/Pill";
import { useClassName } from "hooks/useClassName";
import { getDisplayWorkspaceStatus } from "utils/workspace";

export type WorkspaceStatusBadgeProps = {
  workspace: Workspace;
  children?: ReactNode;
  className?: string;
};

export const WorkspaceStatusBadge: FC<WorkspaceStatusBadgeProps> = ({
  workspace,
  className,
}) => {
  const { text, icon, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
    workspace.latest_build.job,
  );

  return (
    <ChooseOne>
      <Cond condition={workspace.latest_build.status === "failed"}>
        <FailureTooltip
          title={
            <div css={{ display: "flex", alignItems: "center", gap: 10 }}>
              <ErrorOutline
                css={(theme) => ({
                  width: 14,
                  height: 14,
                  color: theme.palette.error.light,
                })}
              />
              <div>{workspace.latest_build.job.error}</div>
            </div>
          }
          placement="top"
        >
          <Pill
            role="status"
            data-testid="build-status"
            className={className}
            icon={icon}
            type={type}
          >
            {text}
          </Pill>
        </FailureTooltip>
      </Cond>
      <Cond>
        <Pill
          role="status"
          data-testid="build-status"
          className={className}
          icon={icon}
          type={type}
        >
          {text}
        </Pill>
      </Cond>
    </ChooseOne>
  );
};

const FailureTooltip: FC<TooltipProps> = ({ children, ...tooltipProps }) => {
  const popper = useClassName(
    (css, theme) => css`
      & .${tooltipClasses.tooltip} {
        background-color: ${theme.palette.background.paper};
        border: 1px solid ${theme.palette.divider};
        font-size: 12px;
        padding: 8px 10px;
      }
    `,
    [],
  );

  return (
    <Tooltip {...tooltipProps} classes={{ popper }}>
      {children}
    </Tooltip>
  );
};
