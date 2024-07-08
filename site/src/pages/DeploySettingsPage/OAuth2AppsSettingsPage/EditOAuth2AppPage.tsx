import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import * as oauth2 from "api/queries/oauth2";
import type * as TypesGen from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { pageTitle } from "utils/page";
import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";

const EditOAuth2AppPage: FC = () => {
  const navigate = useNavigate();
  const { appId } = useParams() as { appId: string };

  // When a new secret is created it is returned with the full secret.  This is
  // the only time it will be visible.  The secret list only returns a truncated
  // version of the secret (for differentiation purposes).  Once the user
  // acknowledges the secret we will clear it from the state.
  const [fullNewSecret, setFullNewSecret] =
    useState<TypesGen.OAuth2ProviderAppSecretFull>();

  const queryClient = useQueryClient();
  const appQuery = useQuery(oauth2.getApp(appId));
  const putAppMutation = useMutation(oauth2.putApp(queryClient));
  const deleteAppMutation = useMutation(oauth2.deleteApp(queryClient));
  const secretsQuery = useQuery(oauth2.getAppSecrets(appId));
  const postSecretMutation = useMutation(oauth2.postAppSecret(queryClient));
  const deleteSecretMutation = useMutation(oauth2.deleteAppSecret(queryClient));

  return (
    <>
      <Helmet>
        <title>{pageTitle("Edit OAuth2 Application")}</title>
      </Helmet>

      <EditOAuth2AppPageView
        app={appQuery.data}
        secrets={secretsQuery.data}
        isLoadingApp={appQuery.isLoading}
        isLoadingSecrets={secretsQuery.isLoading}
        mutatingResource={{
          updateApp: putAppMutation.isLoading,
          deleteApp: deleteAppMutation.isLoading,
          createSecret: postSecretMutation.isLoading,
          deleteSecret: deleteSecretMutation.isLoading,
        }}
        fullNewSecret={fullNewSecret}
        ackFullNewSecret={() => setFullNewSecret(undefined)}
        error={
          appQuery.error ||
          putAppMutation.error ||
          deleteAppMutation.error ||
          secretsQuery.error ||
          postSecretMutation.error ||
          deleteSecretMutation.error
        }
        updateApp={async (req) => {
          try {
            await putAppMutation.mutateAsync({ id: appId, req });
            // REVIEW: Maybe it is better to stay on the same page?
            displaySuccess(
              `Successfully updated the OAuth2 application "${req.name}".`,
            );
            navigate("/deployment/oauth2-provider/apps?updated=true");
          } catch (ignore) {
            displayError("Failed to update OAuth2 application");
          }
        }}
        deleteApp={async (name) => {
          try {
            await deleteAppMutation.mutateAsync(appId);
            displaySuccess(
              `You have successfully deleted the OAuth2 application "${name}"`,
            );
            navigate("/deployment/oauth2-provider/apps?deleted=true");
          } catch (error) {
            displayError("Failed to delete OAuth2 application");
          }
        }}
        generateAppSecret={async () => {
          try {
            const secret = await postSecretMutation.mutateAsync(appId);
            displaySuccess("Successfully generated OAuth2 client secret");
            setFullNewSecret(secret);
          } catch (ignore) {
            displayError("Failed to generate OAuth2 client secret");
          }
        }}
        deleteAppSecret={async (secretId: string) => {
          try {
            await deleteSecretMutation.mutateAsync({ appId, secretId });
            displaySuccess("Successfully deleted an OAuth2 client secret");
            if (fullNewSecret?.id === secretId) {
              setFullNewSecret(undefined);
            }
          } catch (ignore) {
            displayError("Failed to delete OAuth2 client secret");
          }
        }}
      />
    </>
  );
};

export default EditOAuth2AppPage;
