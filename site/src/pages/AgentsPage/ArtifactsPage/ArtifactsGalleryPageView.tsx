import { cn } from "cn";
import {
	CodeXmlIcon,
	FileCodeIcon,
	FileIcon,
	FileTextIcon,
	ImageIcon,
	LayoutGridIcon,
	ListIcon,
	WorkflowIcon,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import type { ChatArtifact } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { SearchField } from "#/components/SearchField/SearchField";
import { timeFrom } from "#/utils/time";
import {
	type ArtifactKind,
	getArtifactPagePath,
	kindOf,
} from "../components/Artifacts/artifactUtils";
import { getTimeGroup, TIME_GROUPS } from "../utils/timeGroups";

type ArtifactFilter = "all" | "yours" | "shared";
type GalleryLayout = "list" | "grid";

const FILTERS: readonly { id: ArtifactFilter; label: string }[] = [
	{ id: "all", label: "All" },
	{ id: "yours", label: "Yours" },
	{ id: "shared", label: "Shared with you" },
];

export const galleryLayoutStorageKey = "agents.artifacts-gallery-layout";

const kindIcons: Record<ArtifactKind, React.FC<{ className?: string }>> = {
	html: CodeXmlIcon,
	mermaid: WorkflowIcon,
	markdown: FileTextIcon,
	image: ImageIcon,
	text: FileCodeIcon,
	download: FileIcon,
};

/**
 * Keeps the artifacts that pass the filter and contain the search text in
 * the title, file name, project name or owner name. The order stays.
 */
export const filterArtifacts = (
	artifacts: readonly ChatArtifact[],
	filter: ArtifactFilter,
	search: string,
	userId: string,
): ChatArtifact[] => {
	const needle = search.trim().toLowerCase();
	return artifacts.filter((artifact) => {
		const isOwn = artifact.owner_id === userId;
		if ((filter === "yours" && !isOwn) || (filter === "shared" && isOwn)) {
			return false;
		}
		return [
			artifact.title,
			artifact.latest_name,
			artifact.project_name,
			artifact.owner_username,
		].some((field) => field.toLowerCase().includes(needle));
	});
};

const readLayout = (): GalleryLayout =>
	localStorage.getItem(galleryLayoutStorageKey) === "grid" ? "grid" : "list";

type ArtifactItemProps = {
	artifact: ChatArtifact;
	currentUserId: string;
	now: Date;
};

const ArtifactListItem: React.FC<ArtifactItemProps> = ({
	artifact,
	currentUserId,
	now,
}) => {
	const Icon =
		kindIcons[kindOf(artifact.latest_name, artifact.latest_media_type)];
	const details = [
		artifact.latest_name,
		`v${artifact.latest_version}`,
		artifact.project_name,
		artifact.owner_id === currentUserId ? "" : `by ${artifact.owner_username}`,
	].filter(Boolean);
	return (
		<li>
			<Link
				to={getArtifactPagePath(artifact.id)}
				className="flex items-center gap-3 rounded-lg px-2 py-2 no-underline hover:bg-surface-secondary"
			>
				<span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-solid border-border bg-surface-secondary text-content-secondary">
					<Icon className="size-4" />
				</span>
				<span className="flex min-w-0 flex-1 flex-col">
					<span className="truncate text-sm font-medium text-content-primary">
						{artifact.title || "Untitled artifact"}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{details.join(" · ")}
					</span>
				</span>
				<span className="shrink-0 text-xs text-content-secondary">
					Edited {timeFrom(artifact.updated_at, now)}
				</span>
			</Link>
		</li>
	);
};

const ArtifactCard: React.FC<ArtifactItemProps> = ({
	artifact,
	currentUserId,
	now,
}) => {
	const Icon =
		kindIcons[kindOf(artifact.latest_name, artifact.latest_media_type)];
	const owner =
		artifact.owner_id === currentUserId ? "" : `by ${artifact.owner_username}`;
	return (
		<li>
			<Link
				to={getArtifactPagePath(artifact.id)}
				className="flex flex-col overflow-hidden rounded-lg border border-solid border-border no-underline hover:bg-surface-secondary"
			>
				<span className="flex h-28 items-center justify-center border-0 border-b border-solid border-border bg-surface-secondary text-content-secondary">
					<Icon className="size-6" />
				</span>
				<span className="flex flex-col gap-0.5 p-3">
					<span className="truncate text-sm font-medium text-content-primary">
						{artifact.title || "Untitled artifact"}
					</span>
					<span className="truncate text-xs text-content-secondary">
						Edited {timeFrom(artifact.updated_at, now)} · v
						{artifact.latest_version}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{[artifact.latest_name, artifact.project_name, owner]
							.filter(Boolean)
							.join(" · ")}
					</span>
				</span>
			</Link>
		</li>
	);
};

type ArtifactsGalleryPageViewProps = {
	/** Undefined while the first load runs. */
	artifacts: readonly ChatArtifact[] | undefined;
	error: unknown;
	currentUserId: string;
	now: Date;
};

/**
 * The artifact gallery, laid out like the Claude Code gallery: All, Yours
 * and Shared with you, a search field, and a list with one section for each
 * time group of the last edit, or a grid of cards.
 */
export const ArtifactsGalleryPageView: React.FC<
	ArtifactsGalleryPageViewProps
> = ({ artifacts, error, currentUserId, now }) => {
	const [filter, setFilter] = useState<ArtifactFilter>("all");
	const [search, setSearch] = useState("");
	const [layout, setLayout] = useState<GalleryLayout>(readLayout);

	const changeLayout = (next: GalleryLayout) => {
		setLayout(next);
		localStorage.setItem(galleryLayoutStorageKey, next);
	};

	// A failed background refetch keeps the cached list on screen.
	let content: React.ReactNode;
	if (!artifacts) {
		content = error ? (
			<ErrorAlert error={error} />
		) : (
			<Loader label="Loading artifacts" />
		);
	} else if (artifacts.length === 0) {
		content = (
			<EmptyState
				message="No artifacts yet"
				description="Files that agents publish from a chat show here."
			/>
		);
	} else {
		const shown = filterArtifacts(artifacts, filter, search, currentUserId);
		if (shown.length === 0) {
			content = (
				<EmptyState
					message="No matching artifacts"
					description="Try another search or filter."
				/>
			);
		} else if (layout === "grid") {
			content = (
				<ul className="m-0 grid list-none grid-cols-1 gap-3 p-0 sm:grid-cols-2 lg:grid-cols-3">
					{shown.map((artifact) => (
						<ArtifactCard
							key={artifact.id}
							artifact={artifact}
							currentUserId={currentUserId}
							now={now}
						/>
					))}
				</ul>
			);
		} else {
			content = TIME_GROUPS.map((name) => {
				const items = shown.filter(
					(artifact) => getTimeGroup(artifact.updated_at, now) === name,
				);
				if (items.length === 0) {
					return null;
				}
				return (
					<section key={name} aria-label={name} className="flex flex-col gap-2">
						<h2 className="m-0 text-xs font-medium text-content-secondary">
							{name}
						</h2>
						<ul className="m-0 flex list-none flex-col gap-0.5 p-0">
							{items.map((artifact) => (
								<ArtifactListItem
									key={artifact.id}
									artifact={artifact}
									currentUserId={currentUserId}
									now={now}
								/>
							))}
						</ul>
					</section>
				);
			});
		}
	}

	return (
		<div className="p-4 pt-8">
			<div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
				<h1 className="m-0 text-2xl font-semibold text-content-primary">
					Artifacts
				</h1>
				<div className="flex flex-wrap items-center gap-2">
					<div
						role="group"
						aria-label="Show artifacts"
						className="flex items-center gap-1"
					>
						{FILTERS.map(({ id, label }) => (
							<Button
								key={id}
								variant="subtle"
								size="sm"
								aria-pressed={filter === id}
								onClick={() => setFilter(id)}
								className={cn(
									"min-w-0",
									filter === id &&
										"bg-surface-quaternary/25 text-content-primary",
								)}
							>
								{label}
							</Button>
						))}
					</div>
					<div className="flex w-full items-center gap-1 sm:ml-auto sm:w-auto">
						<SearchField
							value={search}
							onChange={setSearch}
							placeholder="Search artifacts"
							aria-label="Search artifacts"
							className="min-w-0 flex-1 sm:w-60"
						/>
						<div role="group" aria-label="Layout" className="flex shrink-0">
							<Button
								variant="subtle"
								size="icon"
								aria-label="List view"
								aria-pressed={layout === "list"}
								onClick={() => changeLayout("list")}
								className={cn(layout === "list" && "text-content-primary")}
							>
								<ListIcon />
							</Button>
							<Button
								variant="subtle"
								size="icon"
								aria-label="Grid view"
								aria-pressed={layout === "grid"}
								onClick={() => changeLayout("grid")}
								className={cn(layout === "grid" && "text-content-primary")}
							>
								<LayoutGridIcon />
							</Button>
						</div>
					</div>
				</div>
				{content}
			</div>
		</div>
	);
};
