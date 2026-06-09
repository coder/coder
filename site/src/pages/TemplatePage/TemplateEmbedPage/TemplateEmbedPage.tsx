import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { API } from "#/api/api";
import { DetailedError } from "#/api/errors";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
} from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import {
	createReconnectingWebSocket,
	isCleanClose,
} from "#/utils/reconnectingWebSocket";
import { useTemplateLayoutContext } from "../TemplateLayout";
import { TemplateEmbedPageView } from "./TemplateEmbedPageView";

const TemplateEmbedPage: React.FC = () => {
	const { template } = useTemplateLayoutContext();
	const { user: me } = useAuthenticated();
	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);
	// Reopens the dynamic-parameters socket after an idle (parked) close.
	const reconnectWsRef = useRef<(() => void) | null>(null);
	// The most recent inputs sent to the backend, re-sent on reconnect so the
	// new connection renders against the user's current form state.
	const lastSentInputsRef = useRef<Record<string, string> | null>(null);

	const sendMessage = (formValues: Record<string, string>) => {
		lastSentInputsRef.current = formValues;
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			owner_id: me.id,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
			return;
		}
		// The socket is parked after an idle close. Reopen it; the onOpen
		// handler re-sends the latest inputs once the connection is back.
		reconnectWsRef.current?.();
	};

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		setLatestResponse(response);
	});

	// On reconnect, re-send the latest inputs so the backend renders against
	// the user's current form state.
	const resendCurrentInputs = useEffectEvent(() => {
		if (lastSentInputsRef.current) {
			sendMessage(lastSentInputsRef.current);
		}
	});

	useEffect(() => {
		if (!template.active_version_id || !me) {
			return;
		}

		const handle = createReconnectingWebSocket({
			connect() {
				const socket = API.templateVersionDynamicParameters(
					template.active_version_id,
					me.id,
					{ onMessage },
				);

				ws.current = socket;
				return socket;
			},
			// An idle timeout is a clean close: park the connection instead of
			// reconnecting in a loop, and resume on focus or the next edit.
			shouldReconnect: (event) => !isCleanClose(event),
			resumeOnVisible: true,
			onOpen() {
				setWsError(null);
				resendCurrentInputs();
			},
			onParked() {
				// Idle close is expected; surface no error while parked.
				setWsError(null);
			},
			onDisconnect() {
				setWsError(
					new DetailedError(
						"WebSocket connection for dynamic parameters lost.",
						"Attempting to reconnect...",
					),
				);
			},
		});
		reconnectWsRef.current = handle.reconnect;

		return () => {
			reconnectWsRef.current = null;
			handle.dispose();
		};
	}, [template.active_version_id, me]);

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters]
			.filter((it) => !it.ephemeral)
			.sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	const isLoading =
		ws.current?.readyState === WebSocket.CONNECTING || !latestResponse;

	return (
		<>
			<title>{pageTitle(template.name)}</title>

			<TemplateEmbedPageView
				template={template}
				parameters={sortedParams}
				diagnostics={latestResponse?.diagnostics ?? []}
				error={wsError}
				sendMessage={sendMessage}
				isLoading={isLoading}
			/>
		</>
	);
};

export default TemplateEmbedPage;
