import { useEffect, useState } from "react";
import { type QueryClient, useQuery, useQueryClient } from "react-query";
import { watchACPSession } from "#/api/api";
import { acpSession } from "#/api/queries/acp";
import type { ACPSession } from "#/api/typesGenerated";

type Connection = {
	listeners: Set<(connected: boolean) => void>;
	connected: boolean;
	close: () => void;
};
const connections = new WeakMap<QueryClient, Map<string, Connection>>();

function subscribe(
	client: QueryClient,
	path: string,
	listener: (connected: boolean) => void,
) {
	let pool = connections.get(client);
	if (!pool) {
		pool = new Map();
		connections.set(client, pool);
	}
	let connection = pool.get(path);
	if (!connection) {
		const listeners = new Set<(connected: boolean) => void>();
		let stopped = false;
		let socket: ReturnType<typeof watchACPSession> | undefined;
		let timer: ReturnType<typeof setTimeout> | undefined;
		const state: Connection = {
			listeners,
			connected: false,
			close: () => {
				stopped = true;
				clearTimeout(timer);
				socket?.close();
			},
		};
		const publish = (connected: boolean) => {
			state.connected = connected;
			for (const callback of listeners) callback(connected);
		};
		const connect = () => {
			if (stopped) return;
			socket = watchACPSession(path);
			socket.addEventListener("message", (event) => {
				if (event.parseError) {
					socket?.close();
					return;
				}
				publish(true);
				client.setQueryData<ACPSession | null>(
					acpSession(path).queryKey,
					(previous) =>
						!previous || event.parsedMessage.version >= previous.version
							? event.parsedMessage
							: previous,
				);
			});
			socket.addEventListener("close", () => {
				if (stopped) return;
				publish(false);
				timer = setTimeout(async () => {
					try {
						const snapshot = await client.fetchQuery(acpSession(path));
						if (snapshot === null) return;
					} catch {
						/* The query exposes connection errors to the caller. */
					}
					connect();
				}, 2000);
			});
		};
		connection = state;
		pool.set(path, state);
		connect();
	}
	connection.listeners.add(listener);
	listener(connection.connected);
	return () => {
		connection.listeners.delete(listener);
		if (connection.listeners.size === 0) {
			connection.close();
			pool.delete(path);
		}
	};
}

/** Shares a workspace-agent stream between visible tool cards and the ACP chat. */
export function useACPSession(path: string, enabled = true) {
	const client = useQueryClient();
	const query = useQuery({
		...acpSession(path),
		enabled: enabled && Boolean(path),
	});
	const [connected, setConnected] = useState(false);
	useEffect(() => {
		if (!enabled || !path || query.data === null) return;
		return subscribe(client, path, setConnected);
	}, [client, path, enabled, query.data === null]);
	return { ...query, connected };
}
