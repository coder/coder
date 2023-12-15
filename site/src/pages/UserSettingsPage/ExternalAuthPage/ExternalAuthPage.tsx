import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  externalAuths,
  unlinkExternalAuths,
  validateExternalAuth,
} from "api/queries/externalAuth";
import { getErrorMessage } from "api/errors";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Section } from "../Section";
import { ExternalAuthPageView } from "./ExternalAuthPageView";

const ExternalAuthPage: FC = () => {
  const queryClient = useQueryClient();
  // This is used to tell the child components something was unlinked and things
  // need to be refetched
  const [unlinked, setUnlinked] = useState(0);

  const externalAuthsQuery = useQuery(externalAuths());
  const [appToUnlink, setAppToUnlink] = useState<string>();
  const unlinkAppMutation = useMutation(unlinkExternalAuths(queryClient));
  const validateAppMutation = useMutation(validateExternalAuth(queryClient));

  return (
    <Section title="External Authentication" layout="fluid">
      <ExternalAuthPageView
        isLoading={externalAuthsQuery.isLoading}
        getAuthsError={externalAuthsQuery.error}
        auths={externalAuthsQuery.data}
        unlinked={unlinked}
        onUnlinkExternalAuth={(providerID: string) => {
          setAppToUnlink(providerID);
        }}
        onValidateExternalAuth={async (providerID: string) => {
          try {
            const data = await validateAppMutation.mutateAsync(providerID);
            if (data.authenticated) {
              displaySuccess("Application link is valid.");
            } else {
              displayError(
                "Application link is not valid. Please unlink the application and reauthenticate.",
              );
            }
          } catch (e) {
            displayError(
              getErrorMessage(e, "Error validating application link."),
            );
          }
        }}
      />
      <DeleteDialog
        key={appToUnlink}
        title="Unlink Application"
        verb="Unlinking"
        info="This does not revoke the access token from the oauth2 provider.
        It only removes the link on this side. To fully revoke access, you must
        do so on the oauth2 provider's side."
        label="Name of the application to unlink"
        isOpen={appToUnlink !== undefined}
        confirmLoading={unlinkAppMutation.isLoading}
        name={appToUnlink ?? ""}
        entity="application"
        onCancel={() => setAppToUnlink(undefined)}
        onConfirm={async () => {
          try {
            await unlinkAppMutation.mutateAsync(appToUnlink!);
            // setAppToUnlink closes the modal
            setAppToUnlink(undefined);
            // refetch repopulates the external auth data
            await externalAuthsQuery.refetch();
            // this tells our child components to refetch their data
            // as at least 1 provider was unlinked.
            setUnlinked(unlinked + 1);

            displaySuccess("Successfully unlinked the oauth2 application.");
          } catch (e) {
            displayError(getErrorMessage(e, "Error unlinking application."));
          }
        }}
      />
    </Section>
  );
};

export default ExternalAuthPage;
