import Box from "@mui/material/Box";
import { ProvisionerJobLog } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { useRef, useEffect } from "react";

export const WorkspaceBuildLogsSection = ({
  logs,
}: {
  logs: ProvisionerJobLog[] | undefined;
}) => {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Auto scrolling makes hard to snapshot test using Chromatic
    if (process.env.STORYBOOK === "true") {
      return;
    }

    const scrollEl = scrollRef.current;
    if (scrollEl) {
      scrollEl.scrollTop = scrollEl.scrollHeight;
    }
  }, [logs]);

  return (
    <Box
      sx={(theme) => ({
        borderRadius: 1,
        border: `1px solid ${theme.palette.divider}`,
      })}
    >
      <Box
        sx={(theme) => ({
          background: theme.palette.background.paper,
          borderBottom: `1px solid ${theme.palette.divider}`,
          padding: theme.spacing(1, 1, 1, 3),
          fontSize: 13,
          fontWeight: 600,
          display: "flex",
          alignItems: "center",
          borderRadius: "8px 8px 0 0",
        })}
      >
        Build logs
      </Box>
      <Box
        ref={scrollRef}
        sx={() => ({
          height: "400px",
          overflowY: "auto",
        })}
      >
        {logs ? (
          <WorkspaceBuildLogs
            sticky
            logs={logs}
            sx={{ border: 0, borderRadius: 0 }}
          />
        ) : (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: "100%",
              height: "100%",
            }}
          >
            <Loader />
          </Box>
        )}
      </Box>
    </Box>
  );
};
