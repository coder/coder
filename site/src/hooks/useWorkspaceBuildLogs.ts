import { useEffect, useState } from "react";
import { watchBuildLogsByBuildId } from "#/api/api";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

export const useWorkspaceBuildLogs = (
	// buildId is optional because sometimes the build is not loaded yet
	buildId: string | undefined,
	enabled = true,
) => {
	const [buildLogs, setBuildLogs] = useState<{
		buildId: string;
		logs: ProvisionerJobLog[];
	}>();

	useEffect(() => {
		if (!buildId || !enabled) {
			return;
		}

		let ignore = false;
		const logs: ProvisionerJobLog[] = [];
		const socket = watchBuildLogsByBuildId(buildId, {
			// Retrieve all the logs
			after: -1,
			onMessage: (log) => {
				if (ignore) {
					return;
				}
				logs.push(log);
				setBuildLogs({ buildId, logs: [...logs] });
			},
			onError: () => {
				toast.error(`Error on getting "${buildId}" build logs.`);
			},
		});

		return () => {
			ignore = true;
			socket.close();
		};
	}, [buildId, enabled]);

	// Disabling keeps the last logs. A new build starts from empty logs.
	if (!buildId || !enabled || buildLogs?.buildId === buildId) {
		return buildLogs?.logs;
	}
	return undefined;
};
