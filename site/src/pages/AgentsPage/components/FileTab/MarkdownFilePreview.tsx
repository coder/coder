import { useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { type Components, defaultRehypePlugins } from "streamdown";
import { workspaceAgentFileHead } from "#/api/queries/workspaceFiles";
import { Button } from "#/components/Button/Button";
import { useTheme } from "#/theme/context";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import {
	isWorkspaceImageSource,
	maxPreviewImages,
	previewIdPrefix,
	rehypeFilePreview,
	rehypeRemoveStyleElements,
	resolvePreviewLink,
	resolveWorkspaceUrl,
} from "../../utils/markdownPreview";
import { getPathDirname } from "../../utils/path";
import { MarkdownImage } from "../ChatElements/MarkdownImage";
import { Response } from "../ChatElements/Response";

/**
 * Markdown files larger than this open in Edit: rendering a megabyte of
 * markdown blocks the page for seconds.
 */
export const maxMarkdownPreviewBytes = 256 * 1024;

const maxPreviewImageBytes = 2 * 1024 * 1024;

// GitHub-like raw HTML: rehype-raw parses it and Streamdown's sanitize
// schema (GitHub's allowlist) drops scripts, event handlers, iframes, and
// comments. Streamdown's harden step is left out because it rewrites
// relative links to the Coder origin; sanitizing already blocks unsafe
// protocols.
const previewRehypePlugins = [
	defaultRehypePlugins.raw,
	rehypeRemoveStyleElements,
	defaultRehypePlugins.sanitize,
	rehypeFilePreview,
];

const linkClassName =
	"cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-content-link no-underline hover:underline";

type MarkdownFilePreviewProps = {
	agentId: string;
	/** Absolute path of the previewed file. */
	path: string;
	content: string;
};

/**
 * Renders a workspace markdown file the way GitHub does: sanitized raw
 * HTML, links that open workspace files as tabs, and images loaded from the
 * workspace.
 */
export const MarkdownFilePreview: React.FC<MarkdownFilePreviewProps> = ({
	agentId,
	path,
	content,
}) => {
	const files = useWorkspaceFiles();
	const theme = useTheme();
	const containerRef = useRef<HTMLDivElement>(null);
	// Remote images would tell whoever controls their URL that this viewer
	// opened the file, so each preview loads them only on request.
	const [remoteImagesAllowed, setRemoteImagesAllowed] = useState(false);
	const hasRemoteImages =
		/!\[|<img\b/i.test(content) && /https?:\/\//i.test(content);
	const directory = getPathDirname(path);
	let repoRoot: string | undefined;
	for (const root of files?.repoRoots ?? []) {
		if (path.startsWith(`${root}/`) && root.length > (repoRoot?.length ?? 0)) {
			repoRoot = root;
		}
	}
	// GitHub shows these only in the matching color scheme.
	const hiddenModeSuffix = `#gh-${theme.palette.mode === "dark" ? "light" : "dark"}-mode-only`;

	const scrollToAnchor = (id: string) => {
		const targetId = `${previewIdPrefix}${id}`;
		for (const element of containerRef.current?.querySelectorAll(
			"[id], [name]",
		) ?? []) {
			if (
				element.id === targetId ||
				element.getAttribute("name") === targetId
			) {
				element.scrollIntoView({ block: "start" });
				return;
			}
		}
	};

	const components: Components = {
		a: ({
			href,
			children,
		}: {
			href?: string;
			children?: React.ReactNode;
			node?: unknown;
		}) => {
			if (!href) {
				return <span>{children}</span>;
			}
			if (href.endsWith(hiddenModeSuffix)) {
				return null;
			}
			const link = resolvePreviewLink(href, directory, repoRoot);
			if (link.kind === "external") {
				return (
					<a
						href={href}
						target="_blank"
						rel="noopener noreferrer"
						className={linkClassName}
					>
						{children}
					</a>
				);
			}
			if (link.kind === "anchor") {
				return (
					<a
						href={href}
						className={linkClassName}
						onClick={(event) => {
							event.preventDefault();
							scrollToAnchor(link.id);
						}}
					>
						{children}
					</a>
				);
			}
			if (!files) {
				return <span>{children}</span>;
			}
			return (
				<button
					type="button"
					title={link.path}
					className={linkClassName}
					onClick={() => files.openFile(link.path, { line: link.line })}
				>
					{children}
				</button>
			);
		},
		img: ({
			src,
			alt,
			node,
		}: {
			src?: string | Blob;
			alt?: string;
			node?: { properties?: Record<string, unknown> };
		}) => {
			if (typeof src !== "string" || src.endsWith(hiddenModeSuffix)) {
				return null;
			}
			if (/^https?:\/\//i.test(src)) {
				if (!remoteImagesAllowed) {
					return <ImagePlaceholder alt={alt} reason="blocked" />;
				}
				return (
					<img
						src={src}
						alt={alt ?? ""}
						loading="lazy"
						referrerPolicy="no-referrer"
						className="max-w-full"
					/>
				);
			}
			if (!isWorkspaceImageSource(src)) {
				return <MarkdownImage src={src} alt={alt} />;
			}
			const index = node?.properties?.dataPreviewImage;
			if (typeof index !== "number" || index >= maxPreviewImages) {
				return <ImagePlaceholder alt={alt} reason="not loaded" />;
			}
			return (
				<WorkspaceImage
					agentId={agentId}
					path={resolveWorkspaceUrl(src, directory, repoRoot)}
					alt={alt}
				/>
			);
		},
		// Sources would load without the image checks; the <img> fallback
		// inside each <picture> renders instead.
		source: () => null,
	};

	return (
		<div
			ref={containerRef}
			className="min-h-0 flex-1 overflow-y-auto px-6 py-4"
		>
			{hasRemoteImages && !remoteImagesAllowed && (
				<div className="mb-3 flex items-center gap-3 rounded-md border border-solid border-border bg-surface-secondary px-3 py-1.5 text-xs text-content-secondary">
					<span className="min-w-0 flex-1">
						Remote images are blocked so their hosts do not see your IP address.
					</span>
					<Button
						size="xs"
						variant="outline"
						onClick={() => setRemoteImagesAllowed(true)}
					>
						Load remote images
					</Button>
				</div>
			)}
			{/* Streamdown's block memo ignores new components, so the overrides
			    that read these values remount the rendered markdown. */}
			<Response
				key={`${theme.palette.mode}:${remoteImagesAllowed}:${repoRoot}`}
				rehypePlugins={previewRehypePlugins}
				components={components}
			>
				{content}
			</Response>
		</div>
	);
};

const ImagePlaceholder: React.FC<{ alt?: string; reason: string }> = ({
	alt,
	reason,
}) => (
	<span className="inline-flex rounded-md border border-solid border-border bg-surface-secondary px-2 py-1 text-xs text-content-secondary">
		{alt ? `${alt} (image ${reason})` : `Image ${reason}`}
	</span>
);

type WorkspaceImageProps = {
	agentId: string;
	path: string;
	alt?: string;
};

const WorkspaceImage: React.FC<WorkspaceImageProps> = ({
	agentId,
	path,
	alt,
}) => {
	// One byte past the cap shows the file is too large without trusting
	// listing metadata or reading the whole file.
	const file = useQuery(
		workspaceAgentFileHead(agentId, path, maxPreviewImageBytes + 1),
	);
	const data = file.data;
	const fits =
		data !== undefined && data.data.byteLength <= maxPreviewImageBytes;
	const isImage = data?.contentType.startsWith("image/") ?? false;
	const [url, setUrl] = useState<string>();

	useEffect(() => {
		if (!data || !fits || !isImage) {
			return;
		}
		const objectUrl = URL.createObjectURL(
			new Blob([data.data], { type: data.contentType }),
		);
		setUrl(objectUrl);
		return () => URL.revokeObjectURL(objectUrl);
	}, [data, fits, isImage]);

	if (file.isError) {
		return <ImagePlaceholder alt={alt} reason="not found" />;
	}
	if (data && !fits) {
		return <ImagePlaceholder alt={alt} reason="too large to preview" />;
	}
	if (data && !isImage) {
		return <ImagePlaceholder alt={alt} reason="not an image" />;
	}
	if (!url) {
		return <ImagePlaceholder alt={alt} reason="loading" />;
	}
	return <img src={url} alt={alt ?? ""} className="max-w-full" />;
};
