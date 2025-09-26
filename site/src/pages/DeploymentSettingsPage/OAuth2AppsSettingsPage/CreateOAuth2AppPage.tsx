import { postApp } from "api/queries/oauth2";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";

const CreateOAuth2AppPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const postAppMutation = useMutation(postApp(queryClient));

	return (
		<>
			<title>{pageTitle("New OAuth2 Application")}</title>

			<CreateOAuth2AppPageView
				isUpdating={postAppMutation.isPending}
				error={postAppMutation.error}
				createApp={async (req) => {
					try {
						const app = await postAppMutation.mutateAsync(req);
						displaySuccess(
							`Successfully added the OAuth2 application "${app.name}".`,
						);
						navigate(`/deployment/oauth2-provider/apps/${app.id}?created=true`);
					} catch {
						displayError("Failed to create OAuth2 application");
					}
				}}
			/>
		</>
	);
};

export default CreateOAuth2AppPage;
