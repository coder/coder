import { watchBuildLogsByTemplateVersionId } from "api/api";
import { ProvisionerJobLog, TemplateVersion } from "api/typesGenerated";
import { useState, useEffect } from "react";

export const useWatchVersionLogs = (
  templateVersion: TemplateVersion | undefined,
  options?: { onDone: () => Promise<unknown> },
) => {
  const [logs, setLogs] = useState<ProvisionerJobLog[] | undefined>();
  const templateVersionId = templateVersion?.id;
  const templateVersionStatus = templateVersion?.job.status;

  useEffect(() => {
    setLogs(undefined);
  }, [templateVersionId]);

  useEffect(() => {
    if (!templateVersionId || !templateVersionStatus) {
      return;
    }

    if (templateVersionStatus !== "running") {
      return;
    }

    const socket = watchBuildLogsByTemplateVersionId(templateVersionId, {
      onMessage: (log) => {
        setLogs((logs) => (logs ? [...logs, log] : [log]));
      },
      onDone: options?.onDone,
      onError: (error) => {
        console.error(error);
      },
    });

    return () => {
      socket.close();
    };
  }, [options?.onDone, templateVersionId, templateVersionStatus]);

  return logs;
};
