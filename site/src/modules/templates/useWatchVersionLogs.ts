import { watchBuildLogsByTemplateVersionId } from "api/api";
import type { ProvisionerJobLog, TemplateVersion } from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect, useState } from "react";

export const useWatchVersionLogs = (
	templateVersion: TemplateVersion | undefined,
	options?: { onDone: () => Promise<unknown> },
) => {
	const [logs, setLogs] = useState<ProvisionerJobLog[]>();
	const templateVersionId = templateVersion?.id;
	const [cachedVersionId, setCachedVersionId] = useState(templateVersionId);
	if (cachedVersionId !== templateVersionId) {
		setCachedVersionId(templateVersionId);
		setLogs(undefined);
	}

	const templateVersionStatus = templateVersion?.job.status;
	const stableOnDone = useEffectEvent(() => options?.onDone());
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
			onDone: stableOnDone,
			onError: (error) => {
				console.error(error);
			},
		});

		return () => socket.close();
	}, [stableOnDone, templateVersionId, templateVersionStatus]);

	return logs;
};
