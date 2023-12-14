import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  deleteOAuth2App,
  deleteOAuth2AppSecret,
  postOAuth2AppSecret,
  putOAuth2App,
} from "api/api";
import type * as TypesGen from "api/typesGenerated";
import {
  oauth2App,
  oauth2AppSecrets,
  oauth2AppSecretsKey,
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
  const [newAppSecret, setNewAppSecret] = useState<
    TypesGen.OAuth2AppSecretFull | undefined
  >(undefined);

  const oauth2AppQuery = useQuery(oauth2App(appId));
  const appName = oauth2AppQuery.data?.name;

  const deleteMutation = useMutation({
    mutationFn: deleteOAuth2App,
    onSuccess: async () => {
      displaySuccess(
        `You have successfully deleted the OAuth2 application "${appName}"`,
      );
      navigate("/deployment/oauth2-apps?deleted=true");
    },
    onError: () => displayError("Failed to delete OAuth2 application"),
  });

  const putMutation = useMutation({
    mutationFn: ({
      id,
      req,
    }: {
      id: string;
      req: TypesGen.PutOAuth2AppRequest;
    }) => putOAuth2App(id, req),
    onSuccess: () => {
      displaySuccess(
        `Successfully updated the OAuth2 application "${appName}".`,
      );
      navigate("/deployment/oauth2-apps?updated=true");
    },
    onError: () => displayError("Failed to update OAuth2 application"),
  });

  const oauth2AppSecretsQuery = useQuery(oauth2AppSecrets(appId));

  const postSecretMutation = useMutation({
    mutationFn: postOAuth2AppSecret,
    onSuccess: async (secret: TypesGen.OAuth2AppSecretFull) => {
      displaySuccess("Successfully generated OAuth2 client secret");
      setNewAppSecret(secret);
      await queryClient.invalidateQueries([oauth2AppSecretsKey, appId]);
    },
    onError: () => displayError("Failed to generate OAuth2 client secret"),
  });

  const deleteSecretMutation = useMutation({
    mutationFn: ({ appId, secretId }: { appId: string; secretId: string }) =>
      deleteOAuth2AppSecret(appId, secretId),
    onSuccess: async () => {
      displaySuccess("Successfully deleted an OAuth2 client secret");
      await queryClient.invalidateQueries([oauth2AppSecretsKey, appId]);
    },
    onError: () => displayError("Failed to delete OAuth2 client secret"),
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle("Edit OAuth2 Application")}</title>
      </Helmet>

      <EditOAuth2AppPageView
        app={oauth2AppQuery.data}
        secrets={oauth2AppSecretsQuery.data}
        isLoadingApp={oauth2AppQuery.isLoading}
        isLoadingSecrets={oauth2AppQuery.isLoading}
        mutatingResource={{
          updateApp: putMutation.isLoading,
          deleteApp: deleteMutation.isLoading,
          createSecret: postSecretMutation.isLoading,
          deleteSecret: deleteSecretMutation.isLoading,
        }}
        newAppSecret={newAppSecret}
        dismissNewSecret={() => setNewAppSecret(undefined)}
        error={
          oauth2AppQuery.error ||
          putMutation.error ||
          deleteMutation.error ||
          oauth2AppSecretsQuery.error ||
          postSecretMutation.error ||
          deleteSecretMutation.error
        }
        updateApp={(req: TypesGen.PutOAuth2AppRequest) => {
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
