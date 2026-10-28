import { Navigate, useParams, useSearchParams } from "react-router";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import TemplateVersionEditorPage from "./TemplateVersionEditorPage";

const TemplateFilesEditorPage: React.FC = () => {
	const { experiments } = useDashboard();
	const { organization = "default", template = "" } = useParams();
	const getLink = useLinks();
	const [params] = useSearchParams();
	if (params.get("scope") === "template") {
		if (experiments.includes("template-resources")) {
			return (
				<Navigate
					to={`${getLink(linkToTemplate(organization, template))}/resources`}
					replace
				/>
			);
		}
		const fileParams = new URLSearchParams(params);
		fileParams.delete("scope");
		return <Navigate to={{ search: fileParams.toString() }} replace />;
	}
	return <TemplateVersionEditorPage />;
};

export default TemplateFilesEditorPage;
