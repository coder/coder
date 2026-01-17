import { getErrorMessage } from "api/errors";
import {
	externalAuths,
	unlinkExternalAuths,
	validateExternalAuth,
} from "api/queries/externalAuth";
import type { ExternalAuthLinkProvider } from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Section } from "../Section";
import { ExternalAuthPageView } from "./ExternalAuthPageView";

const ExternalAuthPage: FC = () => {
	const queryClient = useQueryClient();
	// This is used to tell the child components something was unlinked and things
	// need to be refetched
	const [unlinked, setUnlinked] = useState(0);

	const externalAuthsQuery = useQuery(externalAuths());
	const [appToUnlink, setAppToUnlink] = useState<ExternalAuthLinkProvider>();
	const unlinkAppMutation = useMutation(unlinkExternalAuths(queryClient));
	const validateAppMutation = useMutation(validateExternalAuth(queryClient));

	return (
		<Section title="External authentication" layout="fluid">
			<ExternalAuthPageView
				isLoading={externalAuthsQuery.isLoading}
				getAuthsError={externalAuthsQuery.error}
				auths={externalAuthsQuery.data}
				unlinked={unlinked}
				onUnlinkExternalAuth={(provider) => {
					setAppToUnlink(provider);
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
				key={appToUnlink?.id}
				title="Unlink application"
				verb="Unlinking"
				info={
					appToUnlink?.supports_revocation
						? "This action will remove external authentication link and will try to revoke the access token from OAuth2 provider. Auth link will be removed regardless if token revocation is successful."
						: "This action will not revoke the access token from the OAuth2 provider. It only removes the link on this side. To fully revoke access, you must do so on the OAuth2 provider's side."
				}
				label="Name of the application to unlink"
				isOpen={appToUnlink !== undefined}
				confirmLoading={unlinkAppMutation.isPending}
				name={appToUnlink?.id ?? ""}
				entity="application"
				onCancel={() => setAppToUnlink(undefined)}
				onConfirm={async () => {
					try {
						const unlinkResp = await unlinkAppMutation.mutateAsync(
							appToUnlink?.id!,
						);
						// setAppToUnlink closes the modal
						setAppToUnlink(undefined);
						// refetch repopulates the external auth data
						await externalAuthsQuery.refetch();
						// this tells our child components to refetch their data
						// as at least 1 provider was unlinked.
						setUnlinked(unlinked + 1);
						displaySuccess(
							unlinkResp.token_revoked
								? "Successfully deleted external auth link and revoked token from the OAuth2 provider."
								: "Successfully deleted external auth link. Token has NOT been revoked from the OAuth2 provider.",
						);
					} catch (e) {
						displayError(getErrorMessage(e, "Error unlinking application."));
					}
				}}
			/>
		</Section>
	);
};

export default ExternalAuthPage;
