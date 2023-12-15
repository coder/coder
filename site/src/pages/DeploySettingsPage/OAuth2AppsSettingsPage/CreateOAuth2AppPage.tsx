import { useMutation, useQueryClient } from "react-query";
import { postApp } from "api/queries/oauth2";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { useNavigate } from "react-router-dom";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";
import { pageTitle } from "utils/page";
import { Helmet } from "react-helmet-async";

const CreateOAuth2AppPage: FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const postAppMutation = useMutation(postApp(queryClient));

  return (
    <>
      <Helmet>
        <title>{pageTitle("New OAuth2 Application")}</title>
      </Helmet>

      <CreateOAuth2AppPageView
        isUpdating={postAppMutation.isLoading}
        error={postAppMutation.error}
        createApp={async (req) => {
          try {
            const app = await postAppMutation.mutateAsync(req);
            displaySuccess(
              `Successfully added the OAuth2 application "${app.name}".`,
            );
            navigate(`/deployment/oauth2-provider/apps/${app.id}?created=true`);
          } catch (ignore) {
            displayError("Failed to create OAuth2 application");
          }
        }}
      />
    </>
  );
};

export default CreateOAuth2AppPage;
