import type { Workspace } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { type FC, type PropsWithChildren } from "react";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { DormantDeletionText } from "components/WorkspaceDeletion";
import { getDisplayWorkspaceStatus } from "utils/workspace";
import Tooltip, {
  type TooltipProps,
  tooltipClasses,
} from "@mui/material/Tooltip";
import { styled } from "@mui/material/styles";
import Box from "@mui/material/Box";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import { type Interpolation, type Theme } from "@emotion/react";

export type WorkspaceStatusBadgeProps = {
  workspace: Workspace;
  className?: string;
};

export const WorkspaceStatusBadge: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const { text, icon, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
    workspace.latest_build.job,
  );
  return (
    <ChooseOne>
      <Cond condition={workspace.latest_build.status === "failed"}>
        <FailureTooltip
          title={
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.25 }}>
              <ErrorOutline
                sx={{
                  width: 14,
                  height: 14,
                  color: (theme) => theme.palette.error.light,
                }}
              />
              <Box>{workspace.latest_build.job.error}</Box>
            </Box>
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

export const WorkspaceStatusText: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const { text, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
  );

  return (
    <ChooseOne>
      {/* <DormantDeletionText/> determines its own visibility */}
      <Cond condition={Boolean(DormantDeletionText({ workspace }))}>
        <DormantDeletionText workspace={workspace} />
      </Cond>
      <Cond>
        <span
          role="status"
          data-testid="build-status"
          className={className}
          css={[styles.root, styles[`type-${type}`]]}
        >
          {text}
        </span>
      </Cond>
    </ChooseOne>
  );
};

const FailureTooltip = styled(({ className, ...props }: TooltipProps) => (
  <Tooltip {...props} classes={{ popper: className }} />
))(({ theme }) => ({
  [`& .${tooltipClasses.tooltip}`]: {
    backgroundColor: theme.palette.background.paperLight,
    border: `1px solid ${theme.palette.divider}`,
    fontSize: 12,
    padding: "8px 10px",
  },
}));

const styles = {
  root: { fontWeight: 600 },

  "type-error": (theme) => ({
    color: theme.palette.error.light,
  }),
  "type-warning": (theme) => ({
    color: theme.palette.warning.light,
  }),
  "type-success": (theme) => ({
    color: theme.palette.success.light,
  }),
  "type-info": (theme) => ({
    color: theme.palette.info.light,
  }),
  "type-undefined": (theme) => ({
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
