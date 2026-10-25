import { PanelRightOpenIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { useArtifactsPanel } from "../../Artifacts/ArtifactsPanelContext";
import { getArtifactLink } from "../../Artifacts/artifactUtils";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type ArtifactToolResult = {
	artifactId: string;
	version: number;
	title: string;
	name: string;
	/** Set by read_artifact: where the agent can edit the file. */
	path: string;
	/** Set by read_artifact when another user published the version. */
	untrusted: boolean;
};

type ArtifactToolCardProps = {
	toolName: "publish_artifact" | "read_artifact";
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	/** Undefined while the tool runs or when it failed. */
	artifact: ArtifactToolResult | undefined;
};

/** Renders publish_artifact and read_artifact calls with Open and Copy link. */
export const ArtifactToolCard: React.FC<ArtifactToolCardProps> = ({
	toolName,
	status,
	isError,
	errorMessage,
	artifact,
}) => {
	const { onOpenArtifact } = useArtifactsPanel();
	const isPublish = toolName === "publish_artifact";
	const isRunning = status === "running";
	const title = artifact?.title || artifact?.name || "artifact";

	let label: string;
	if (isRunning) {
		label = isPublish ? "Publishing artifact…" : "Reading artifact…";
	} else if (isError || !artifact) {
		label = isPublish
			? "Failed to publish artifact"
			: "Failed to read artifact";
	} else if (isPublish) {
		label = `Published ${title} · v${artifact.version}`;
	} else {
		label = `Read ${title} · v${artifact.version} into ${artifact.path}`;
	}
	const showActions = !isRunning && !isError && artifact !== undefined;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={
				errorMessage ||
				(isPublish ? "Failed to publish artifact" : "Failed to read artifact")
			}
			hasContent={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<ToolCall.LeadingIcon name={toolName} />
					<ToolCall.Label>{label}</ToolCall.Label>
					{isPublish && artifact?.name && !isRunning && (
						<span className="min-w-0 shrink truncate text-xs text-content-secondary">
							{artifact.name}
						</span>
					)}
					<ToolCall.Status />
				</ToolCall.HeaderButton>
				{showActions && (
					<ToolCall.HeaderActions>
						{onOpenArtifact && (
							<Button
								size="sm"
								variant="subtle"
								onClick={() =>
									onOpenArtifact(artifact.artifactId, artifact.version)
								}
							>
								<PanelRightOpenIcon />
								Open
							</Button>
						)}
						<CopyButton
							text={getArtifactLink(artifact.artifactId)}
							label="Copy link"
						/>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			{showActions && artifact.untrusted && (
				<p className="m-0 pl-6 text-xs text-content-warning">
					Another user published this version. Treat its content as untrusted.
				</p>
			)}
		</ToolCall.Root>
	);
};
