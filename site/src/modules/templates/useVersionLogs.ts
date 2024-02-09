import { watchBuildLogsByTemplateVersionId } from "api/api";
import {
  ProvisionerJobLog,
  ProvisionerJobStatus,
  TemplateVersion,
} from "api/typesGenerated";
import { useState, useEffect } from "react";

export const useVersionLogs = (
  templateVersion: TemplateVersion | undefined,
  options?: { onDone: () => Promise<unknown> },
) => {
  const [logs, setLogs] = useState<ProvisionerJobLog[]>();
  const templateVersionId = templateVersion?.id;
  const templateVersionStatus = templateVersion?.job.status;

  useEffect(() => {
    const enabledStatuses: ProvisionerJobStatus[] = ["running", "pending"];

    if (!templateVersionId || !templateVersionStatus) {
      return;
    }

    if (!enabledStatuses.includes(templateVersionStatus)) {
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

  return {
    logs,
    setLogs,
  };
};
