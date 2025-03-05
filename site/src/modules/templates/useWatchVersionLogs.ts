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
		setLogs([]);
	}

	const stableOnDone = useEffectEvent(() => options?.onDone());
	const status = templateVersion?.job.status;
	const canWatch = status === "running" || status === "pending";
	useEffect(() => {
		if (!templateVersionId || !canWatch) {
			return;
		}

		const socket = watchBuildLogsByTemplateVersionId(templateVersionId, {
			onError: (error) => console.error(error),
			onDone: stableOnDone,
			onMessage: (newLog) => {
				setLogs((current) => [...(current ?? []), newLog]);
			},
		});

		return () => socket.close();
	}, [stableOnDone, canWatch, templateVersionId]);

	return logs;
};
