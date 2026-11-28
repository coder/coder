import escapeHTML from "lodash/escape";
import { RefreshCwIcon } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { previewAddress, previewSignIn } from "#/api/queries/previews";
import type {
	PreviewSignInRequest,
	PreviewSignInResponse,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useProxy } from "#/contexts/ProxyContext";

// The message a preview host's signed-out page posts to the dashboard, from
// coderd/workspaceapps/preview.go.
const previewSignInNeeded = "coder-preview-sign-in-needed";

type PreviewFrameProps = {
	/** The preview to show, through the selected workspace proxy. */
	preview: Omit<PreviewSignInRequest, "region_id">;
	title: string;
};

/**
 * Shows a preview in a wrapper frame that keeps it on its own origin. The
 * frame loads the preview with the browser's preview session and signs in
 * only when the preview host says the session is missing.
 */
export const PreviewFrame: React.FC<PreviewFrameProps> = ({
	preview,
	title,
}) => {
	const { proxy } = useProxy();
	const request = { ...preview, region_id: proxy.proxy?.id };
	const addressQuery = useQuery(previewAddress(request));
	const address = addressQuery.data;
	const [reloadCount, setReloadCount] = useState(0);

	return (
		<div className="flex size-full min-h-0 flex-col">
			<div className="flex shrink-0 flex-wrap items-center gap-x-2 border-b bg-surface-primary px-3 py-1 text-xs">
				{address && (
					<span className="min-w-0 max-w-full truncate font-medium text-content-primary">
						{new URL(address.origin).host}
					</span>
				)}
				<Button
					size="icon"
					variant="subtle"
					className="ml-auto"
					aria-label="Reload preview"
					onClick={() => {
						setReloadCount((count) => count + 1);
						if (addressQuery.isError) {
							void addressQuery.refetch();
						}
					}}
				>
					<RefreshCwIcon />
				</Button>
			</div>
			{address ? (
				<SignInOnRequestFrame
					key={reloadCount}
					address={address}
					request={request}
					title={title}
				/>
			) : addressQuery.error ? (
				<div className="p-4">
					<ErrorAlert error={addressQuery.error} />
				</div>
			) : (
				<PreviewLoading />
			)}
		</div>
	);
};

type SignInOnRequestFrameProps = {
	address: PreviewSignInResponse;
	request: PreviewSignInRequest;
	title: string;
};

const SignInOnRequestFrame: React.FC<SignInOnRequestFrameProps> = ({
	address,
	request,
	title,
}) => {
	const frameRef = useRef<HTMLIFrameElement>(null);
	const signInRequested = useRef(false);
	const signInMutation = useMutation(previewSignIn());

	// Script in the preview can post the same message too, so each mount
	// signs in at most once.
	const handleMessage = useEffectEvent((event: MessageEvent) => {
		if (
			event.data !== previewSignInNeeded ||
			event.origin !== address.origin ||
			event.source !== frameRef.current?.contentWindow?.frames[0] ||
			signInRequested.current
		) {
			return;
		}
		signInRequested.current = true;
		signInMutation.mutate(request);
	});
	useEffect(() => {
		const listener = (event: MessageEvent) => handleMessage(event);
		window.addEventListener("message", listener);
		return () => window.removeEventListener("message", listener);
	}, []);

	if (signInMutation.error) {
		return (
			<div className="p-4">
				<ErrorAlert error={signInMutation.error} />
			</div>
		);
	}
	if (signInMutation.isPending) {
		return <PreviewLoading />;
	}
	const frame = signInMutation.data ?? address;
	return (
		<iframe
			ref={frameRef}
			srcDoc={wrapperHTML(frame, title)}
			title={title}
			sandbox={frame.sandbox}
			className="min-h-0 w-full flex-1"
		/>
	);
};

const PreviewLoading: React.FC = () => (
	<div className="flex min-h-0 flex-1 items-center justify-center">
		<Spinner loading label="Loading preview" />
	</div>
);

// The preview's own CSP cannot stop it from navigating itself to another app,
// so the wrapper's frame-src keeps it on its origin. style-src allows only the
// inline style that sizes the frame, which default-src 'none' would block.
const wrapperHTML = (
	{ url, origin }: PreviewSignInResponse,
	title: string,
): string => {
	const policy = `default-src 'none'; style-src 'unsafe-inline'; frame-src ${origin}`;
	return [
		"<!doctype html>",
		`<meta http-equiv="Content-Security-Policy" content="${escapeHTML(policy)}">`,
		`<iframe src="${escapeHTML(url)}" title="${escapeHTML(title)}"`,
		' referrerpolicy="no-referrer"',
		' style="position:fixed;inset:0;width:100%;height:100%;border:0">',
		"</iframe>",
	].join("");
};
