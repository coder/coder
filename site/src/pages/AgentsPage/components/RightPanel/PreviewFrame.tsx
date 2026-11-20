import escapeHTML from "lodash/escape";
import { RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { previewSignIn } from "#/api/queries/previews";
import type {
	PreviewSignInRequest,
	PreviewSignInResponse,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useProxy } from "#/contexts/ProxyContext";
import { generateUUID } from "#/utils/random";

type PreviewFrameProps = {
	/** The preview to sign in to, through the selected workspace proxy. */
	preview: Omit<PreviewSignInRequest, "region_id">;
	/** Accessible name of the frame. */
	title: string;
	/** Shown after the preview's host in the header. */
	children?: React.ReactNode;
};

/**
 * Signs in to a preview and shows it in a wrapper frame that keeps it on its
 * own origin, under a header with the preview's host and a reload button.
 * Each mount and each reload signs in again.
 */
export const PreviewFrame: React.FC<PreviewFrameProps> = ({
	preview,
	title,
	children,
}) => {
	const { proxy } = useProxy();
	const [frameId, setFrameId] = useState(generateUUID);
	const signInQuery = useQuery(
		previewSignIn({ ...preview, region_id: proxy.proxy?.id }, frameId),
	);
	const signIn = signInQuery.data;

	return (
		<div className="flex size-full min-h-0 flex-col">
			<div className="flex min-h-8 shrink-0 flex-wrap items-center gap-x-2 border-b bg-surface-primary px-3 py-1 text-xs">
				{signIn && (
					<span className="min-w-0 max-w-full truncate font-medium text-content-primary">
						{new URL(signIn.origin).host}
					</span>
				)}
				{children}
				<Button
					size="icon"
					variant="subtle"
					className="ml-auto"
					aria-label="Reload preview"
					onClick={() => setFrameId(generateUUID())}
				>
					<RefreshCwIcon />
				</Button>
			</div>
			{signInQuery.error ? (
				<div className="p-4">
					<ErrorAlert error={signInQuery.error} />
				</div>
			) : signIn ? (
				<iframe
					srcDoc={wrapperHTML(signIn, title)}
					title={title}
					sandbox={signIn.sandbox}
					className="min-h-0 w-full flex-1"
				/>
			) : (
				<div className="flex min-h-0 flex-1 items-center justify-center">
					<Spinner loading label="Loading preview" />
				</div>
			)}
		</div>
	);
};

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
