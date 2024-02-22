import Drawer from "@mui/material/Drawer";
import Close from "@mui/icons-material/Close";
import IconButton from "@mui/material/IconButton";
import { visuallyHidden } from "@mui/utils";
import { FC, useEffect, useRef } from "react";
import { TemplateVersion } from "api/typesGenerated";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { useTheme } from "@emotion/react";
import { navHeight } from "theme/constants";
import { useVersionLogs } from "modules/templates/useVersionLogs";
import { JobError } from "api/queries/templates";
import AlertTitle from "@mui/material/AlertTitle";
import { Alert, AlertDetail } from "components/Alert/Alert";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";

type BuildLogsDrawerProps = {
  error: unknown;
  open: boolean;
  onClose: () => void;
  templateVersion: TemplateVersion | undefined;
  variablesSectionRef: React.RefObject<HTMLDivElement>;
};

export const BuildLogsDrawer: FC<BuildLogsDrawerProps> = ({
  templateVersion,
  error,
  variablesSectionRef,
  ...drawerProps
}) => {
  const theme = useTheme();
  const { logs } = useVersionLogs(templateVersion);
  const logsContainer = useRef<HTMLDivElement>(null);

  const scrollToBottom = () => {
    setTimeout(() => {
      if (logsContainer.current) {
        logsContainer.current.scrollTop = logsContainer.current.scrollHeight;
      }
    }, 0);
  };

  useEffect(() => {
    scrollToBottom();
  }, [logs]);

  useEffect(() => {
    if (drawerProps.open) {
      scrollToBottom();
    }
  }, [drawerProps.open]);

  const isMissingVariables =
    error instanceof JobError &&
    error.job.error_code === "REQUIRED_TEMPLATE_VARIABLES";

  return (
    <Drawer anchor="right" {...drawerProps}>
      <div
        css={{
          width: 800,
          height: "100%",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <header
          css={{
            height: navHeight,
            padding: "0 24px",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: `1px solid ${theme.palette.divider}`,
          }}
        >
          <h3 css={{ margin: 0, fontWeight: 500, fontSize: 16 }}>
            Creating template...
          </h3>
          <IconButton size="small" onClick={drawerProps.onClose}>
            <Close css={{ fontSize: 20 }} />
            <span style={visuallyHidden}>Close build logs</span>
          </IconButton>
        </header>

        <Collapse in={isMissingVariables}>
          <Alert
            css={{
              borderTop: 0,
              borderRight: 0,
              backgroundColor: theme.palette.background.paper,
              borderRadius: 0,
              borderBottomColor: theme.palette.divider,
              paddingLeft: 24,
            }}
            severity="warning"
            actions={
              <Button
                size="small"
                variant="text"
                onClick={() => {
                  variablesSectionRef.current?.scrollIntoView({
                    behavior: "smooth",
                  });
                  const firstVariableInput =
                    variablesSectionRef.current?.querySelector("input");
                  setTimeout(() => firstVariableInput?.focus(), 0);
                  drawerProps.onClose();
                }}
              >
                Add variables
              </Button>
            }
          >
            <AlertTitle>Failed to create template</AlertTitle>
            <AlertDetail>{isMissingVariables && error.message}</AlertDetail>
          </Alert>
        </Collapse>
        <section
          ref={logsContainer}
          css={{
            flex: 1,
            overflow: "auto",
            backgroundColor: theme.palette.background.default,
          }}
        >
          <WorkspaceBuildLogs logs={logs ?? []} css={{ border: 0 }} />
        </section>
      </div>
    </Drawer>
  );
};
