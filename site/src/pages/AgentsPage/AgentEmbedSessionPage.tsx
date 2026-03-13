import { getErrorMessage } from "api/errors";
import { bootstrapChatEmbedSession } from "api/queries/users";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { permissionChecks } from "modules/permissions";
import { type FC, useCallback, useEffect, useRef } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";

type BootstrapMessage = {
	type: "coder:vscode-auth-bootstrap";
	payload: {
		token: string;
	};
};

const getBootstrapToken = (data: unknown): string | undefined => {
	if (typeof data !== "object" || data === null) {
		return undefined;
	}

	const message = data as Partial<BootstrapMessage>;
	if (message.type !== "coder:vscode-auth-bootstrap") {
		return undefined;
	}

	if (typeof message.payload !== "object" || message.payload === null) {
		return undefined;
	}

	const payload = message.payload as { token?: unknown };
	if (typeof payload.token !== "string") {
		return undefined;
	}

	const token = payload.token.trim();
	return token.length > 0 ? token : undefined;
};

const EmbedStatusView: FC<{
	message: string;
	label: string;
}> = ({ message, label }) => {
	return (
		<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
			<Loader label={label} />
			<p className="max-w-md text-sm text-content-secondary">{message}</p>
		</div>
	);
};

const EmbedErrorView: FC<{
	error: unknown;
	onRetry: () => void;
}> = ({ error, onRetry }) => {
	return (
		<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
			<div className="space-y-2">
				<h1 className="text-xl font-semibold text-content-primary">
					Unable to start embedded agent.
				</h1>
				<p className="max-w-md text-sm text-content-secondary">
					{getErrorMessage(
						error,
						"We couldn't exchange the VS Code bootstrap token for a session.",
					)}
				</p>
			</div>
			<Button onClick={onRetry}>Try again</Button>
		</div>
	);
};

const AgentEmbedSessionPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	if (!agentId) {
		throw new Error(
			"AgentEmbedSessionPage requires an agentId route parameter.",
		);
	}

	const auth = useAuthContext();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const embedSessionMutation = useMutation(
		bootstrapChatEmbedSession({ checks: permissionChecks }, queryClient),
	);
	const latestEmbedSessionMutationRef = useRef(embedSessionMutation);
	latestEmbedSessionMutationRef.current = embedSessionMutation;
	const inFlightBootstrapRef = useRef<Promise<unknown> | null>(null);
	const embedPath = `/agents/${agentId}/embed`;

	useEffect(() => {
		if (!auth.isSignedIn) {
			return;
		}
		navigate(embedPath, { replace: true });
	}, [auth.isSignedIn, embedPath, navigate]);

	const isAwaitingBootstrapMessage =
		auth.isSignedOut &&
		!embedSessionMutation.isPending &&
		!embedSessionMutation.isError;

	useEffect(() => {
		if (!isAwaitingBootstrapMessage) {
			return;
		}

		const parentWindow = window.parent;

		const handleMessage = (event: MessageEvent) => {
			if (event.source !== parentWindow) {
				return;
			}

			const token = getBootstrapToken(event.data);
			if (!token || inFlightBootstrapRef.current) {
				return;
			}

			const bootstrapPromise = latestEmbedSessionMutationRef.current
				.mutateAsync(token)
				.catch(() => undefined)
				.finally(() => {
					inFlightBootstrapRef.current = null;
				});
			inFlightBootstrapRef.current = bootstrapPromise;
		};

		// Register the listener before notifying the parent so an immediate
		// bootstrap response is never missed.
		window.addEventListener("message", handleMessage);
		parentWindow.postMessage(
			{ type: "coder:vscode-ready", payload: { agentId } },
			"*",
		);
		return () => {
			window.removeEventListener("message", handleMessage);
		};
	}, [agentId, isAwaitingBootstrapMessage]);

	const handleRetry = useCallback(() => {
		inFlightBootstrapRef.current = null;
		embedSessionMutation.reset();
	}, [embedSessionMutation]);

	if (embedSessionMutation.isError) {
		return (
			<EmbedErrorView
				error={embedSessionMutation.error}
				onRetry={handleRetry}
			/>
		);
	}

	if (embedSessionMutation.isPending) {
		return (
			<EmbedStatusView
				label="Signing in to embedded agent"
				message="Signing in to the embedded agent…"
			/>
		);
	}

	if (isAwaitingBootstrapMessage) {
		return (
			<EmbedStatusView
				label="Waiting for VS Code authentication"
				message="Waiting for VS Code authentication…"
			/>
		);
	}

	return (
		<EmbedStatusView
			label="Loading embedded agent session"
			message="Loading embedded agent session…"
		/>
	);
};

export default AgentEmbedSessionPage;
