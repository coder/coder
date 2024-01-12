import { watchBuildLogsByBuildId } from "api/api";
import { ProvisionerJobLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useState, useEffect } from "react";

export const useWorkspaceBuildLogs = (
  // buildId is optional because sometimes the build is not loaded yet
  buildId: string | undefined,
  enabled: boolean = true,
) => {
  const [logs, setLogs] = useState<ProvisionerJobLog[]>();
  useEffect(() => {
    if (!buildId || !enabled) {
      return;
    }

    // Every time this hook is called reset the values
    setLogs(undefined);

    const socket = watchBuildLogsByBuildId(buildId, {
      // Retrieve all the logs
      after: -1,
      onMessage: (log) => {
        setLogs((previousLogs) => {
          if (!previousLogs) {
            return [log];
          }
          return [...previousLogs, log];
        });
      },
      onError: () => {
        displayError("Error on getting the build logs");
      },
    });

    return () => {
      socket.close();
    };
  }, [buildId]);

  return logs;
};
