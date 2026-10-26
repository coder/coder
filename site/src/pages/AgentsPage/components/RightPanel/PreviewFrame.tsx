import escapeHTML from "lodash/escape";
import { useState } from "react";
import type { ChatPreview, Workspace } from "#/api/typesGenerated";
import { Spinner } from "#/components/Spinner/Spinner";
import { workspaceAppURL } from "#/utils/portForward";

export type PreviewIdentity = Pick<
	ChatPreview,
	"agent_name" | "port" | "path" | "url_path"
>;

/**
 * Identifies a preview in the Select and the saved selection. Must match how
 * chatPreviewsFromResults in coderd merges results.
 */
export const previewKey = (preview: PreviewIdentity): string =>
	JSON.stringify(
		// A file preview gets a new ID when its agent restarts.
		preview.path
			? [preview.agent_name, preview.path]
			: [preview.agent_name, preview.port, preview.url_path ?? ""],
	);

/** Returns undefined when the deployment has no wildcard access URL. */
export const previewURL = (
	preview: ChatPreview,
	workspace: Workspace,
	wildcardHostname: string,
): string | undefined => {
	const urlPath = preview.url_path ?? "";
	const queryStart = urlPath.indexOf("?");
	const pathname = queryStart === -1 ? urlPath : urlPath.slice(0, queryStart);
	const search = queryStart === -1 ? undefined : urlPath.slice(queryStart);
	const url = workspaceAppURL(
		wildcardHostname,
		`preview--${preview.preview_id || preview.port}`,
		preview.agent_name,
		workspace.name,
		workspace.owner_name,
		pathname,
		search,
	);
	return url === "#" ? undefined : url;
};

/** Returns a document that frames url and lets it navigate only within its origin. */
export const previewWrapperHTML = (url: string, title: string): string => {
	// The preview's own CSP can't stop it from navigating itself to another
	// app, so a wrapper page's frame-src keeps it on its own origin.
	const policy = `frame-src ${new URL(url).origin}`;
	return [
		"<!doctype html>",
		`<meta http-equiv="Content-Security-Policy" content="${escapeHTML(policy)}">`,
		`<iframe src="${escapeHTML(url)}" title="${escapeHTML(title)}"`,
		' referrerpolicy="no-referrer"',
		' style="position:fixed;inset:0;width:100%;height:100%;border:0">',
		"</iframe>",
	].join("");
};

type PreviewFrameProps = {
	url: string;
	title: string;
};

/** Renders a preview that can only navigate within its own origin. */
export const PreviewFrame: React.FC<PreviewFrameProps> = ({ url, title }) => {
	const origin = new URL(url).origin;
	const [signInLoadedOrigin, setSignInLoadedOrigin] = useState<string>();

	// The wrapper's frame-src blocks the redirects that sign the browser in to
	// apps, so a hidden frame follows them first. The param matches
	// PreviewSignInParam in coderd/workspaceapps/proxy.go.
	if (signInLoadedOrigin !== origin) {
		return (
			<div className="flex size-full items-center justify-center">
				<Spinner loading label="Loading preview" />
				<iframe
					src={`${origin}/?coder_preview_sign_in`}
					title="Signing in to the preview"
					sandbox=""
					hidden
					onLoad={() => setSignInLoadedOrigin(origin)}
				/>
			</div>
		);
	}

	return (
		// Matches previewSandbox in coderd/workspaceapps/proxy.go. The
		// wrapper's frame inherits these flags.
		<iframe
			srcDoc={previewWrapperHTML(url, title)}
			title={title}
			sandbox="allow-scripts allow-same-origin allow-forms allow-modals allow-downloads"
			className="size-full border-0"
		/>
	);
};
