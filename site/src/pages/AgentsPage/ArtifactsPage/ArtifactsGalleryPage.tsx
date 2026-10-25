import { useQuery } from "react-query";
import { Link } from "react-router";
import { chatArtifacts } from "#/api/queries/chatArtifacts";
import type { ChatArtifact } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { relativeTime } from "#/utils/time";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { getArtifactPagePath } from "../components/Artifacts/artifactUtils";

type ArtifactGroup = { name: string; artifacts: ChatArtifact[] };

/**
 * Groups artifacts for the gallery: "Mine" first (the user owns them and no
 * project shares them), then one group for each project, by project name.
 */
export const groupArtifacts = (
	artifacts: readonly ChatArtifact[],
	userId: string,
): ArtifactGroup[] => {
	const mine: ChatArtifact[] = [];
	const byProject = new Map<string, ArtifactGroup>();
	for (const artifact of artifacts) {
		if (!artifact.project_id) {
			// Without a project only the owner can see an artifact.
			if (artifact.owner_id === userId) {
				mine.push(artifact);
			}
			continue;
		}
		const group = byProject.get(artifact.project_id) ?? {
			name: artifact.project_name || "Unnamed project",
			artifacts: [],
		};
		group.artifacts.push(artifact);
		byProject.set(artifact.project_id, group);
	}
	const projects = [...byProject.values()].sort((a, b) =>
		a.name.localeCompare(b.name, "en"),
	);
	return mine.length > 0
		? [{ name: "Mine", artifacts: mine }, ...projects]
		: projects;
};

const ArtifactsGalleryPage: React.FC = () => {
	const { user } = useAuthenticated();
	const artifactsQuery = useQuery(chatArtifacts());

	// A failed background refetch keeps the cached list on screen.
	let content: React.ReactNode;
	if (!artifactsQuery.data) {
		content = artifactsQuery.isError ? (
			<ErrorAlert error={artifactsQuery.error} />
		) : (
			<Loader label="Loading artifacts" />
		);
	} else {
		const groups = groupArtifacts(artifactsQuery.data, user.id);
		content =
			groups.length === 0 ? (
				<EmptyState
					message="No artifacts yet"
					description="Files that agents publish from a chat show here."
				/>
			) : (
				groups.map((group) => (
					<section key={group.name} aria-label={group.name}>
						<h2 className="m-0 mb-2 text-sm font-medium text-content-primary">
							{group.name}
						</h2>
						<ul className="m-0 flex list-none flex-col gap-1 p-0">
							{group.artifacts.map((artifact) => (
								<li key={artifact.id}>
									<Link
										to={getArtifactPagePath(artifact.id)}
										className="flex items-center gap-3 rounded-md px-3 py-2 text-sm text-content-primary no-underline hover:bg-surface-secondary"
									>
										<span className="min-w-0 flex-1 truncate">
											{artifact.title || "Untitled artifact"}
										</span>
										<span className="shrink-0 text-xs text-content-secondary">
											v{artifact.latest_version} · {artifact.owner_username} ·{" "}
											{relativeTime(artifact.updated_at)}
										</span>
									</Link>
								</li>
							))}
						</ul>
					</section>
				))
			);
	}

	return (
		<>
			<title>{pageTitle("Artifacts", "Agents")}</title>
			<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
				<AgentPageHeader mobileBack={{ to: "/agents", label: "Agents" }} />
				<div className="p-4 pt-8">
					<div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
						<h1 className="m-0 text-xl font-semibold text-content-primary">
							Artifacts
						</h1>
						{content}
					</div>
				</div>
			</ScrollArea>
		</>
	);
};

export default ArtifactsGalleryPage;
