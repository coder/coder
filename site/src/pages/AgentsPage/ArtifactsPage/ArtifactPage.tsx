import { ArrowLeftIcon } from "lucide-react";
import { useQuery } from "react-query";
import {
	Link,
	useLocation,
	useNavigate,
	useParams,
	useSearchParams,
} from "react-router";
import { getErrorStatus } from "#/api/errors";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { ArtifactView } from "../components/Artifacts/ArtifactView";
import {
	artifactsGalleryPath,
	findShownVersion,
} from "../components/Artifacts/artifactUtils";

/**
 * The gallery URL that opened this page, with its filter and search, or the
 * plain gallery.
 */
const galleryPathFromState = (state: unknown): string => {
	const from: unknown =
		typeof state === "object" && state !== null && "from" in state
			? state.from
			: undefined;
	const isGalleryPath =
		typeof from === "string" &&
		(from === artifactsGalleryPath ||
			from.startsWith(`${artifactsGalleryPath}?`));
	return isGalleryPath ? from : artifactsGalleryPath;
};

/**
 * One artifact on its own page. It shows the latest version unless the
 * version search parameter names another one. There is no chat here, so a
 * message from an HTML version is not sent, and a toast says so and offers
 * to open the chat of the version.
 */
const ArtifactPage: React.FC = () => {
	const { artifactId = "" } = useParams();
	const { user } = useAuthenticated();
	const navigate = useNavigate();
	const galleryPath = galleryPathFromState(useLocation().state);
	const [searchParams, setSearchParams] = useSearchParams();
	const versionParam = Number(searchParams.get("version"));
	const selectedVersion =
		Number.isInteger(versionParam) && versionParam > 0
			? versionParam
			: undefined;
	const artifactQuery = useQuery({
		...chatArtifact(artifactId),
		enabled: artifactId !== "",
	});

	// A failed background refetch keeps the cached artifact on screen.
	let content: React.ReactNode;
	if (artifactQuery.data) {
		const artifact = artifactQuery.data;
		const showPromptToast = () => {
			const version = findShownVersion(artifact, selectedVersion);
			// The chat title is set only when the viewer owns the chat, so
			// only then can the viewer open it.
			const chatId = version?.chat_title ? version.chat_id : undefined;
			toast.info("Open this artifact in its chat to send messages from it.", {
				action: chatId
					? {
							label: "Open chat",
							onClick: () => navigate(`/agents/${chatId}`),
						}
					: undefined,
			});
		};
		content = (
			<ArtifactView
				artifact={artifact}
				selectedVersion={selectedVersion}
				onSelectVersion={(version) =>
					setSearchParams({ version: String(version) }, { replace: true })
				}
				onPrompt={showPromptToast}
				currentUserId={user.id}
				leading={
					<Button
						asChild
						variant="subtle"
						size="icon"
						className="hidden sm:inline-flex"
					>
						<Link to={galleryPath} aria-label="Back to artifacts">
							<ArrowLeftIcon />
						</Link>
					</Button>
				}
			/>
		);
	} else if (artifactQuery.isError) {
		// The API answers 404 for an artifact the viewer cannot see, and 400
		// for an ID that is not a UUID. Both mean the link leads nowhere.
		const status = getErrorStatus(artifactQuery.error);
		content =
			status === 404 || status === 400 ? (
				<EmptyState
					message="Artifact not found"
					description="The artifact does not exist, or you do not have access to it. Ask its owner to publish it from a chat in a project that you are a member of."
					cta={
						<Button asChild variant="outline">
							<Link to={galleryPath}>
								<ArrowLeftIcon />
								Back to artifacts
							</Link>
						</Button>
					}
				/>
			) : (
				<div className="p-4">
					<ErrorAlert error={artifactQuery.error} />
				</div>
			);
	} else {
		content = <Loader className="flex-1" label="Loading artifact" />;
	}

	return (
		<>
			<title>
				{pageTitle(artifactQuery.data?.title || "Artifact", "Agents")}
			</title>
			<div className="flex min-h-0 flex-1 flex-col">
				<AgentPageHeader
					mobileBack={{ to: galleryPath, label: "Back to artifacts" }}
				/>
				{content}
			</div>
		</>
	);
};

export default ArtifactPage;
