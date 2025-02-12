import { watchBuildLogsByTemplateVersionId } from "api/api";
import type { ProvisionerJobLog, TemplateVersion } from "api/typesGenerated";
import { useEffect, useState } from "react";

export const useWatchVersionLogs = (
	templateVersion: TemplateVersion | undefined,
	options?: { onDone: () => Promise<unknown> },
) => {
	const [logs, setLogs] = useState<ProvisionerJobLog[] | undefined>();
	const templateVersionId = templateVersion?.id;
	const templateVersionStatus = templateVersion?.job.status;

	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useEffect(() => {
		setLogs(undefined);
	}, [templateVersionId]);

	useEffect(() => {
		if (!templateVersionId || !templateVersionStatus) {
			return;
		}

		if (
			templateVersionStatus !== "running" &&
			templateVersionStatus !== "pending"
		) {
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
