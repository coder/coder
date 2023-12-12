import { useMutation } from "react-query";
import { postOAuth2App } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { useNavigate } from "react-router-dom";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";
import { pageTitle } from "utils/page";
import { Helmet } from "react-helmet-async";

const CreateOAuth2AppPage: FC = () => {
  const navigate = useNavigate();

  const postMutation = useMutation({
    mutationFn: postOAuth2App,
    onSuccess: (newApp: TypesGen.OAuth2App) => {
      displaySuccess(
        `Successfully added the OAuth2 application "${newApp.name}".`,
      );
      navigate(`/deployment/oauth2-apps/${newApp.id}?created=true`);
    },
    onError: () => displayError("Failed to create OAuth2 application"),
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle("New OAuth2 Application")}</title>
      </Helmet>

      <CreateOAuth2AppPageView
        isUpdating={postMutation.isLoading}
        error={postMutation.error}
        createApp={(req: TypesGen.PostOAuth2AppRequest) => {
          postMutation.mutate(req);
        }}
      />
    </>
  );
};

export default CreateOAuth2AppPage;
