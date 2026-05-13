import type { FC } from "react";
import { pageTitle } from "#/utils/page";

const ListProvidersPage: FC = () => {
	// TODO(jakehwll): wire up the providers list view.
	// See https://linear.app/codercom/project/coder-agents-ai-gov-unification
	// for the AI Bridge provider CRUD work this page hangs off of.
	return (
		<>
			<title>{pageTitle("Providers", "AI Bridge")}</title>
			<p>TODO: list AI Bridge providers.</p>
		</>
	);
};

export default ListProvidersPage;
