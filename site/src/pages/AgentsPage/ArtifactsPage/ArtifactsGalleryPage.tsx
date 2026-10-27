import { useQuery } from "react-query";
import { chatArtifacts } from "#/api/queries/chatArtifacts";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { ArtifactsGalleryPageView } from "./ArtifactsGalleryPageView";

const ArtifactsGalleryPage: React.FC = () => {
	const { user } = useAuthenticated();
	const artifactsQuery = useQuery(chatArtifacts());

	return (
		<>
			<title>{pageTitle("Artifacts", "Agents")}</title>
			<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
				<AgentPageHeader mobileBack={{ to: "/agents", label: "Agents" }} />
				<ArtifactsGalleryPageView
					artifacts={artifactsQuery.data}
					error={artifactsQuery.error}
					currentUserId={user.id}
					now={new Date()}
				/>
			</ScrollArea>
		</>
	);
};

export default ArtifactsGalleryPage;
