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

type BuildLogsDrawerProps = {
  open: boolean;
  onClose: () => void;
  templateVersion: TemplateVersion | undefined;
};

export const BuildLogsDrawer: FC<BuildLogsDrawerProps> = ({
  templateVersion,
  ...drawerProps
}) => {
  const theme = useTheme();
  const { logs } = useVersionLogs(templateVersion);

  // Auto scroll
  const logsContainer = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (logsContainer.current) {
      logsContainer.current.scrollTop = logsContainer.current.scrollHeight;
    }
  }, [logs]);

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
            padding: "0 20px",
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
