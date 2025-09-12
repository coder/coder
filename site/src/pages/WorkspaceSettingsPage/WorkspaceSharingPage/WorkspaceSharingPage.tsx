import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";

const WorkspaceSharingPage: FC = () => {
	const workspace = useWorkspaceSettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle(workspace.name, "Sharing")}</title>
			</Helmet>
			<PageHeader className="pt-0">
				<PageHeaderTitle>Sharing</PageHeaderTitle>
			</PageHeader>
		</>
	);
};

export default WorkspaceSharingPage;
