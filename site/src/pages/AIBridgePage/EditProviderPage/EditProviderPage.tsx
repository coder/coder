import type { FC } from "react";
import { pageTitle } from "#/utils/page";

const EditProviderPage: FC = () => {
	// TODO(jakehwll): wire up the edit provider form.
	// See https://linear.app/codercom/project/coder-agents-ai-gov-unification
	// for the AI Bridge provider CRUD work this page hangs off of.
	return (
		<>
			<title>{pageTitle("Edit Provider", "AI Bridge")}</title>
			<p>TODO: edit an existing AI Bridge provider.</p>
		</>
	);
};

export default EditProviderPage;
