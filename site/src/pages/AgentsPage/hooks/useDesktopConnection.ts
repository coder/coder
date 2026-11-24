import { useEffect, useState, useSyncExternalStore } from "react";
import { toast } from "#/components/Toaster/toast";
import { useClipboard } from "#/hooks/useClipboard";
import {
	DesktopConnection,
	type DesktopConnectionStatus,
} from "../utils/desktopConnection";

export type { DesktopConnectionStatus } from "../utils/desktopConnection";

type UseDesktopConnectionOptions = {
	chatId: string | undefined;
	/** When false the hook stays dormant, no WebSocket, no RFB. */
	activated: boolean;
	/** Whether the viewport starts scaled to fit the container. Default: false (native 100%). */
	initialScaleViewport?: boolean;
};

export type UseDesktopConnectionResult = {
	/** Current connection status. */
	status: DesktopConnectionStatus;
	/** Whether the connection has ever been established. */
	hasConnected: boolean;
	/** Whether the viewport is scaled to fit the container. */
	scaleViewport: boolean;
	/** Scales the viewport to fit the container, or shows it at 100%. */
	setScaleViewport: (scaleViewport: boolean) => void;
	/**
	 * Tear down the current connection and start a fresh one.
	 * Used by the "Reconnect" button after an error.
	 */
	reconnect: () => void;
	/**
	 * Attach the noVNC canvas to a container element. Can be called
	 * multiple times (e.g., when the tab is re-selected). The RFB
	 * instance moves its existing canvas into the new container
	 * without reconnecting.
	 */
	attach: (container: HTMLElement) => void;
};

export function useDesktopConnection({
	chatId,
	activated,
	initialScaleViewport = false,
}: UseDesktopConnectionOptions): UseDesktopConnectionResult {
	const [connection] = useState(
		() => new DesktopConnection(initialScaleViewport),
	);
	const { status, hasConnected, scaleViewport } = useSyncExternalStore(
		connection.subscribe,
		connection.getSnapshot,
	);
	const { copyToClipboard: syncRemoteClipboardToLocal } = useClipboard({
		onError: () => {
			toast.error(
				"Failed to sync the remote clipboard to your local clipboard.",
			);
		},
	});

	useEffect(() => {
		if (!activated || !chatId) {
			return;
		}

		connection.open(chatId, syncRemoteClipboardToLocal);
		return connection.close;
	}, [connection, activated, chatId, syncRemoteClipboardToLocal]);

	return {
		status,
		hasConnected,
		scaleViewport,
		setScaleViewport: connection.setScaleViewport,
		reconnect: connection.reconnect,
		attach: connection.attach,
	};
}
