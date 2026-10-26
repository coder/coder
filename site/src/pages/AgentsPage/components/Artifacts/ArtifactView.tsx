import {
	DownloadIcon,
	ExternalLinkIcon,
	MessageSquareIcon,
} from "lucide-react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import { getChatArtifactVersionURL } from "#/api/api";
import { chatArtifactVersionText } from "#/api/queries/chatArtifacts";
import type { ChatArtifact, ChatArtifactVersion } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Response } from "../ChatElements/Response";
import { ArtifactFrame } from "./ArtifactFrame";
import { ArtifactShareButton } from "./ArtifactShareButton";
import {
	describeArtifactVersion,
	findShownVersion,
	getArtifactPagePath,
	getVersionChatPath,
	kindOf,
} from "./artifactUtils";

type ArtifactViewProps = {
	artifact: ChatArtifact;
	/** The version to show. The latest version shows when it is unset or unknown. */
	selectedVersion: number | undefined;
	onSelectVersion: (version: number) => void;
	/** Names versions from this chat "this chat". Unset outside a chat. */
	currentChatId?: string;
	/** Receives accepted prompts from an HTML version. */
	onPrompt?: (text: string) => void;
	/** The viewer, for the Share popover. */
	currentUserId: string;
	/** Shown before the title, such as a back link. */
	leading?: React.ReactNode;
	/** Replaces the plain title, such as with a menu that switches artifacts. */
	title?: React.ReactNode;
	/** Adds a link that opens the artifact page in a new tab. */
	showPageLink?: boolean;
	/**
	 * Adds a link to the chat that published the latest version, when the
	 * viewer can open that chat.
	 */
	showChatLink?: boolean;
};

/**
 * One artifact: a version menu, Download, Share, and the rendered version.
 */
export const ArtifactView: React.FC<ArtifactViewProps> = ({
	artifact,
	selectedVersion,
	onSelectVersion,
	currentChatId,
	onPrompt,
	currentUserId,
	leading,
	title,
	showPageLink = false,
	showChatLink = false,
}) => {
	const versions = artifact.versions ?? [];
	const version = findShownVersion(artifact, selectedVersion);
	// Versions are newest first.
	const latestVersion = versions[0];
	const chatPath =
		showChatLink && latestVersion
			? getVersionChatPath(latestVersion)
			: undefined;

	return (
		<div className="flex h-full min-h-0 flex-1 flex-col">
			<div className="@container flex shrink-0 flex-wrap items-center gap-2 border-0 border-b border-solid border-border px-3 py-2">
				{leading}
				{/* In a narrow header, such as a phone or the chat panel, the title
				    takes its own row, so it is not cut off. */}
				<h3 className="m-0 flex min-w-0 basis-full text-sm font-medium text-content-primary @xl:flex-1">
					{title ?? (
						<span className="min-w-0 truncate">
							{artifact.title || "Untitled artifact"}
						</span>
					)}
				</h3>
				{chatPath && latestVersion && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button asChild variant="outline" size="sm">
								<Link to={chatPath}>
									<MessageSquareIcon />
									Open chat
								</Link>
							</Button>
						</TooltipTrigger>
						<TooltipContent>
							Last edited in “{latestVersion.chat_title}”
						</TooltipContent>
					</Tooltip>
				)}
				{version && (
					<Select
						value={String(version.version)}
						onValueChange={(value) => onSelectVersion(Number(value))}
					>
						<SelectTrigger
							aria-label="Artifact version"
							// In a narrow header the menu shrinks, so Download and Share
							// stay on its row.
							className="h-8 w-auto min-w-0 max-w-72 flex-1 text-xs @xl:flex-none"
						>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{versions.map((v) => (
								<SelectItem key={v.version} value={String(v.version)}>
									{describeArtifactVersion(v, currentChatId)}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				)}
				{version && (
					<Button asChild variant="subtle" size="icon">
						<a
							href={getChatArtifactVersionURL(artifact.id, version.version)}
							download={version.name}
							aria-label="Download"
						>
							<DownloadIcon />
						</a>
					</Button>
				)}
				{showPageLink && (
					<Button asChild variant="subtle" size="icon">
						<a
							href={getArtifactPagePath(artifact.id, version?.version)}
							target="_blank"
							rel="noreferrer"
							aria-label="Open artifact page"
						>
							<ExternalLinkIcon />
						</a>
					</Button>
				)}
				<ArtifactShareButton
					artifact={artifact}
					version={version?.version}
					currentUserId={currentUserId}
				/>
			</div>
			{version ? (
				<ArtifactVersionContent
					artifactId={artifact.id}
					version={version}
					onPrompt={onPrompt}
				/>
			) : (
				<p className="m-0 p-4 text-sm text-content-secondary">
					This artifact has no versions.
				</p>
			)}
		</div>
	);
};

type ArtifactVersionContentProps = {
	artifactId: string;
	version: ChatArtifactVersion;
	onPrompt?: (text: string) => void;
};

const ArtifactVersionContent: React.FC<ArtifactVersionContentProps> = ({
	artifactId,
	version,
	onPrompt,
}) => {
	const kind = kindOf(version.name, version.media_type);
	const url = getChatArtifactVersionURL(artifactId, version.version);
	switch (kind) {
		case "html":
			return (
				<ArtifactFrame
					artifactId={artifactId}
					version={version.version}
					title={version.name}
					onPrompt={onPrompt}
				/>
			);
		case "image":
			return (
				<div className="min-h-0 flex-1 overflow-auto p-4">
					{/* An SVG in an img element runs no script. */}
					<img src={url} alt={version.name} className="max-w-full" />
				</div>
			);
		case "markdown":
		case "mermaid":
		case "text":
			return (
				<ArtifactText artifactId={artifactId} version={version} kind={kind} />
			);
		case "download":
			return (
				<div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-sm text-content-secondary">
					<span>{version.name} cannot be previewed.</span>
					<Button asChild variant="outline" size="sm">
						<a href={url} download={version.name}>
							<DownloadIcon />
							Download
						</a>
					</Button>
				</div>
			);
		default: {
			const _exhaustive: never = kind;
			return _exhaustive;
		}
	}
};

type ArtifactTextProps = {
	artifactId: string;
	version: ChatArtifactVersion;
	kind: "markdown" | "mermaid" | "text";
};

const ArtifactText: React.FC<ArtifactTextProps> = ({
	artifactId,
	version,
	kind,
}) => {
	const textQuery = useQuery(
		chatArtifactVersionText(artifactId, version.version),
	);
	if (textQuery.isError) {
		return (
			<div className="p-4">
				<ErrorAlert error={textQuery.error} />
			</div>
		);
	}
	if (textQuery.data === undefined) {
		return <Loader className="flex-1" label="Loading artifact" />;
	}
	const text = textQuery.data;
	return (
		<div className="min-h-0 flex-1 overflow-auto p-4">
			{kind === "text" ? (
				<pre className="m-0 whitespace-pre-wrap break-words font-mono text-xs text-content-primary">
					<code>{formatArtifactText(text, version.media_type)}</code>
				</pre>
			) : (
				// Response renders a mermaid code fence with MermaidDiagram.
				<Response>
					{kind === "mermaid" ? `\`\`\`mermaid\n${text}\n\`\`\`` : text}
				</Response>
			)}
		</div>
	);
};

const formatArtifactText = (text: string, mediaType: string): string => {
	if (mediaType !== "application/json") {
		return text;
	}
	try {
		return JSON.stringify(JSON.parse(text), null, 2);
	} catch {
		// Invalid JSON shows as published.
		return text;
	}
};
