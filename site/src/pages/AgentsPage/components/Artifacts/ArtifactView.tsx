import { DownloadIcon } from "lucide-react";
import { useQuery } from "react-query";
import { getChatArtifactVersionURL } from "#/api/api";
import { chatArtifactVersionText } from "#/api/queries/chatArtifacts";
import type { ChatArtifact, ChatArtifactVersion } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { Loader } from "#/components/Loader/Loader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Response } from "../ChatElements/Response";
import { ArtifactFrame } from "./ArtifactFrame";
import {
	describeArtifactVersion,
	getArtifactLink,
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
};

/** One artifact: a version menu, Copy link, and the rendered version. */
export const ArtifactView: React.FC<ArtifactViewProps> = ({
	artifact,
	selectedVersion,
	onSelectVersion,
	currentChatId,
	onPrompt,
}) => {
	const versions = artifact.versions ?? [];
	const version =
		versions.find((v) => v.version === selectedVersion) ?? versions[0];

	return (
		<div className="flex h-full min-h-0 flex-1 flex-col">
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border px-3 py-2">
				<h3 className="m-0 min-w-0 flex-1 truncate text-sm font-medium text-content-primary">
					{artifact.title || "Untitled artifact"}
				</h3>
				{version && (
					<Select
						value={String(version.version)}
						onValueChange={(value) => onSelectVersion(Number(value))}
					>
						<SelectTrigger
							aria-label="Artifact version"
							className="h-8 w-auto max-w-72 text-xs"
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
				<CopyButton
					text={getArtifactLink(artifact.id)}
					label="Copy link"
					tooltipSide="bottom"
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
