import { watchBuildLogsByBuildId } from "api/api";
import { ProvisionerJobLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useState, useEffect } from "react";

// buildId is optional because sometimes the build is not loaded yet
export const useWorkspaceBuildLogs = (buildId?: string) => {
  const [logs, setLogs] = useState<ProvisionerJobLog[]>();
  useEffect(() => {
    if (!buildId) {
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
