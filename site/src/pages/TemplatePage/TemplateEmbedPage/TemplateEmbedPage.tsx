import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { API } from "#/api/api";
import { DetailedError } from "#/api/errors";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
} from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
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

	const sendMessage = (formValues: Record<string, string>) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			owner_id: me.id,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	};

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		setLatestResponse(response);
	});

	useEffect(() => {
		if (!template.active_version_id || !me) {
			return;
		}

		const socket = API.templateVersionDynamicParameters(
			template.active_version_id,
			me.id,
			{
				onMessage,
				onError: (error) => {
					if (ws.current === socket) {
						setWsError(error);
					}
				},
				onClose: () => {
					if (ws.current === socket) {
						setWsError(
							new DetailedError(
								"Websocket connection for dynamic parameters unexpectedly closed.",
								"Refresh the page to reset the form.",
							),
						);
					}
				},
			},
		);

		ws.current = socket;

		return () => {
			socket.close();
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
