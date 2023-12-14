import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  deleteOAuth2ProviderApp,
  deleteOAuth2ProviderAppSecret,
  postOAuth2ProviderAppSecret,
  putOAuth2ProviderApp,
} from "api/api";
import type * as TypesGen from "api/typesGenerated";
import {
  oauth2ProviderApp,
  oauth2ProviderAppSecrets,
  oauth2ProviderAppSecretsKey,
} from "api/queries/oauth2";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";
import { pageTitle } from "utils/page";
import { Helmet } from "react-helmet-async";

const EditOAuth2AppPage: FC = () => {
  const navigate = useNavigate();
  const { appId } = useParams() as { appId: string };
  const queryClient = useQueryClient();
  // When a new secret is created it is returned with the full secret.  This is
  // the only time it will be visible.  The secret list only returns a truncated
  // version of the secret (for differentiation purposes).  Once the user
  // acknowledges the secret we will clear it from the state.
  const [fullNewSecret, setFullNewSecret] =
    useState<TypesGen.OAuth2ProviderAppSecretFull>();

  const appQuery = useQuery(oauth2ProviderApp(appId));
  const appName = appQuery.data?.name;

  const deleteMutation = useMutation({
    mutationFn: deleteOAuth2ProviderApp,
    onSuccess: async () => {
      displaySuccess(
        `You have successfully deleted the OAuth2 application "${appName}"`,
      );
      navigate("/deployment/oauth2-provider/apps?deleted=true");
    },
    onError: () => displayError("Failed to delete OAuth2 application"),
  });

  const putMutation = useMutation({
    mutationFn: ({
      id,
      req,
    }: {
      id: string;
      req: TypesGen.PutOAuth2ProviderAppRequest;
    }) => putOAuth2ProviderApp(id, req),
    onSuccess: () => {
      displaySuccess(
        `Successfully updated the OAuth2 application "${appName}".`,
      );
      navigate("/deployment/oauth2-provider/apps?updated=true");
    },
    onError: () => displayError("Failed to update OAuth2 application"),
  });

  const secretsQuery = useQuery(oauth2ProviderAppSecrets(appId));

  const postSecretMutation = useMutation({
    mutationFn: postOAuth2ProviderAppSecret,
    onSuccess: async (secret: TypesGen.OAuth2ProviderAppSecretFull) => {
      displaySuccess("Successfully generated OAuth2 client secret");
      setFullNewSecret(secret);
      await queryClient.invalidateQueries([oauth2ProviderAppSecretsKey, appId]);
    },
    onError: () => displayError("Failed to generate OAuth2 client secret"),
  });

  const deleteSecretMutation = useMutation({
    mutationFn: ({ appId, secretId }: { appId: string; secretId: string }) =>
      deleteOAuth2ProviderAppSecret(appId, secretId),
    onSuccess: async () => {
      displaySuccess("Successfully deleted an OAuth2 client secret");
      await queryClient.invalidateQueries([oauth2ProviderAppSecretsKey, appId]);
    },
    onError: () => displayError("Failed to delete OAuth2 client secret"),
  });

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
          updateApp: putMutation.isLoading,
          deleteApp: deleteMutation.isLoading,
          createSecret: postSecretMutation.isLoading,
          deleteSecret: deleteSecretMutation.isLoading,
        }}
        fullNewSecret={fullNewSecret}
        ackFullNewSecret={() => setFullNewSecret(undefined)}
        error={
          appQuery.error ||
          putMutation.error ||
          deleteMutation.error ||
          secretsQuery.error ||
          postSecretMutation.error ||
          deleteSecretMutation.error
        }
        updateApp={(req: TypesGen.PutOAuth2ProviderAppRequest) => {
          putMutation.mutate({ id: appId, req });
        }}
        deleteApp={() => {
          deleteMutation.mutate(appId);
        }}
        generateAppSecret={() => {
          postSecretMutation.mutate(appId);
        }}
        deleteAppSecret={(secretId: string) => {
          deleteSecretMutation.mutate({ appId, secretId });
        }}
      />
    </>
  );
};

export default EditOAuth2AppPage;
