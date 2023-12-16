import { useTheme } from "@emotion/react";
import { type FC, useRef, useEffect } from "react";
import type { ProvisionerJobLog } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs";

interface WorkspaceBuildLogsSectionProps {
  logs?: ProvisionerJobLog[];
}

export const WorkspaceBuildLogsSection: FC<WorkspaceBuildLogsSectionProps> = ({
  logs,
}) => {
  const scrollRef = useRef<HTMLDivElement>(null);
  const theme = useTheme();

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
    <div
      css={{
        borderRadius: 8,
        border: `1px solid ${theme.palette.divider}`,
        overflow: "hidden",
      }}
    >
      <header
        css={{
          background: theme.palette.background.paper,
          borderBottom: `1px solid ${theme.palette.divider}`,
          padding: "8px 8px 8px 24px",
          fontSize: 13,
          fontWeight: 600,
          display: "flex",
          alignItems: "center",
          borderRadius: "8px 8px 0 0",
        }}
      >
        Build logs
      </header>
      <div ref={scrollRef} css={{ height: "400px", overflowY: "auto" }}>
        {logs ? (
          <WorkspaceBuildLogs
            sticky
            logs={logs}
            css={{ border: 0, borderRadius: 0 }}
          />
        ) : (
          <div
            css={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: "100%",
              height: "100%",
            }}
          >
            <Loader />
          </div>
        )}
      </div>
    </div>
  );
};
