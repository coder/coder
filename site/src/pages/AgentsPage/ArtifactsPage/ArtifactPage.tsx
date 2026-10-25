import { useQuery } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { ArtifactView } from "../components/Artifacts/ArtifactView";

/**
 * One artifact on its own page. It shows the latest version unless the
 * version search parameter names another one. There is no chat here, so
 * messages from an HTML version do nothing.
 */
const ArtifactPage: React.FC = () => {
	const { artifactId = "" } = useParams();
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

	let content: React.ReactNode;
	if (artifactQuery.isError) {
		content = (
			<div className="p-4">
				<ErrorAlert error={artifactQuery.error} />
			</div>
		);
	} else if (!artifactQuery.data) {
		content = <Loader className="flex-1" label="Loading artifact" />;
	} else {
		content = (
			<ArtifactView
				artifact={artifactQuery.data}
				selectedVersion={selectedVersion}
				onSelectVersion={(version) =>
					setSearchParams({ version: String(version) }, { replace: true })
				}
			/>
		);
	}

	return (
		<>
			<title>
				{pageTitle(artifactQuery.data?.title || "Artifact", "Agents")}
			</title>
			<div className="flex min-h-0 flex-1 flex-col">
				<AgentPageHeader
					mobileBack={{ to: "/agents/artifacts", label: "Artifacts" }}
				/>
				{content}
			</div>
		</>
	);
};

export default ArtifactPage;
