import { Workspace } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { FC, PropsWithChildren } from "react";
import { makeStyles } from "@mui/styles";
import { combineClasses } from "utils/combineClasses";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { ImpendingDeletionText } from "components/WorkspaceDeletion";
import { getDisplayWorkspaceStatus } from "utils/workspace";
import Tooltip, { TooltipProps, tooltipClasses } from "@mui/material/Tooltip";
import { styled } from "@mui/material/styles";
import Box from "@mui/material/Box";
import ErrorOutline from "@mui/icons-material/ErrorOutline";

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
  const styles = useStyles();
  const { text, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
  );

  return (
    <ChooseOne>
      {/* <ImpendingDeletionText/> determines its own visibility */}
      <Cond condition={Boolean(ImpendingDeletionText({ workspace }))}>
        <ImpendingDeletionText workspace={workspace} />
      </Cond>
      <Cond>
        <span
          role="status"
          data-testid="build-status"
          className={combineClasses([
            className,
            styles.root,
            styles[`type-${type}`],
          ])}
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
    padding: theme.spacing(1, 1.25),
  },
}));

const useStyles = makeStyles((theme) => ({
  root: { fontWeight: 600 },
  "type-error": {
    color: theme.palette.error.light,
  },
  "type-warning": {
    color: theme.palette.warning.light,
  },
  "type-success": {
    color: theme.palette.success.light,
  },
  "type-info": {
    color: theme.palette.info.light,
  },
  "type-undefined": {
    color: theme.palette.text.secondary,
  },
  "type-primary": {
    color: theme.palette.text.primary,
  },
  "type-secondary": {
    color: theme.palette.text.secondary,
  },
}));
