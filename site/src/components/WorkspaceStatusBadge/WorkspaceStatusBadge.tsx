import Tooltip, {
  type TooltipProps,
  tooltipClasses,
} from "@mui/material/Tooltip";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import { type FC, type ReactNode } from "react";
import type { Workspace } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { DormantDeletionText } from "components/WorkspaceDeletion";
import { getDisplayWorkspaceStatus } from "utils/workspace";
import { useClassName } from "hooks/useClassName";

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
          <div>
            <Pill className={className} icon={icon} text={text} type={type} />
          </div>
        </FailureTooltip>
      </Cond>
      <Cond>
        <Pill className={className} icon={icon} text={text} type={type} />
      </Cond>
    </ChooseOne>
  );
};

export const WorkspaceStatusText: FC<WorkspaceStatusBadgeProps> = ({
  workspace,
  className,
}) => {
  const { text, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
  );

  return (
    <ChooseOne>
      <Cond condition={Boolean(DormantDeletionText({ workspace }))}>
        <DormantDeletionText workspace={workspace} />
      </Cond>
      <Cond>
        <span
          role="status"
          data-testid="build-status"
          className={className}
          css={(theme) => ({
            fontWeight: 600,
            color: type
              ? theme.experimental.roles[type].fill
              : theme.experimental.l1.text,
          })}
        >
          {text}
        </span>
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
