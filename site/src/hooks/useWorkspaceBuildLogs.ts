import isEqual from "lodash/isEqual";
import { useEffect, useRef, useState } from "react";
import { watchBuildLogsByBuildId } from "#/api/api";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

export const useWorkspaceBuildLogs = (
	// buildId is optional because sometimes the build is not loaded yet
	buildId: string | undefined,
	enabled = true,
) => {
	// The logs are tagged with their build so that messages from a socket
	// that is about to close cannot leak into the next build's logs.
	const [buildLogs, setBuildLogs] = useState<{
		buildId: string | undefined;
		logs: ProvisionerJobLog[] | undefined;
	}>({ buildId, logs: undefined });
	const socket = useRef<WebSocket>(undefined);

	// Every new watch starts from empty logs. Disabling keeps the logs.
	const watch = { buildId, enabled };
	const [prevWatch, setPrevWatch] = useState(watch);
	if (!isEqual(watch, prevWatch)) {
		setPrevWatch(watch);
		if (buildId && enabled) {
			setBuildLogs({ buildId, logs: undefined });
		}
	}

	useEffect(() => {
		if (!buildId || !enabled) {
			socket.current?.close();
			return;
		}

		socket.current = watchBuildLogsByBuildId(buildId, {
			// Retrieve all the logs
			after: -1,
			onMessage: (log) => {
				setBuildLogs((previous) => {
					if (previous.buildId !== buildId) {
						return previous;
					}
					return { buildId, logs: [...(previous.logs ?? []), log] };
				});
			},
			onError: () => {
				toast.error(`Error on getting "${buildId}" build logs.`);
			},
		});

		return () => {
			socket.current?.close();
		};
	}, [buildId, enabled]);

	return buildLogs.logs;
};
