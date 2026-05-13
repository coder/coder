import type { FC } from "react";
import { pageTitle } from "#/utils/page";

const CreateProviderPage: FC = () => {
	// TODO(jakehwll): wire up the create provider form.
	// See https://linear.app/codercom/project/coder-agents-ai-gov-unification
	// for the AI Bridge provider CRUD work this page hangs off of.
	return (
		<>
			<title>{pageTitle("Create Provider", "AI Bridge")}</title>
			<p>TODO: create a new AI Bridge provider.</p>
		</>
	);
};

export default CreateProviderPage;
