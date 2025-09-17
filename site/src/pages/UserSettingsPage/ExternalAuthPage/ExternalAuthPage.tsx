import { getErrorMessage } from "api/errors";
import {
	externalAuths,
	unlinkExternalAuths,
	validateExternalAuth,
} from "api/queries/externalAuth";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Section } from "../Section";
import { ExternalAuthPageView } from "./ExternalAuthPageView";

const TryRevokeInfo =
	"This action will remove external authentication link and will try to revoke the access token from OAuth2 provider." +
	" Auth link will be removed regardless if token revocation is successful.";
const NoRevokeInfo =
	"This action will not revoke the access token from the OAuth2 provider." +
	" It only removes the link on this side. To fully revoke access, you must" +
	" do so on the OAuth2 provider's side.";

const RevokeSuccess =
	"Successfully deleted external auth link and revoked token from the OAuth2 provider.";
const RevokeFailed =
	"Successfully deleted external auth link. Token has NOT been revoked from the OAuth2 provider.";

const ExternalAuthPage: FC = () => {
	const queryClient = useQueryClient();
	// This is used to tell the child components something was unlinked and things
	// need to be refetched
	const [unlinked, setUnlinked] = useState(0);

	const externalAuthsQuery = useQuery(externalAuths());
	const [appToUnlink, setAppToUnlink] = useState<string>();
	const [appSupportsRevoke, setAppSupportsRevoke] = useState<boolean>();
	const unlinkAppMutation = useMutation(unlinkExternalAuths(queryClient));
	const validateAppMutation = useMutation(validateExternalAuth(queryClient));

	return (
		<Section title="External Authentication" layout="fluid">
			<ExternalAuthPageView
				isLoading={externalAuthsQuery.isLoading}
				getAuthsError={externalAuthsQuery.error}
				auths={externalAuthsQuery.data}
				unlinked={unlinked}
				onUnlinkExternalAuth={(
					providerID: string,
					supports_revocation: boolean,
				) => {
					setAppToUnlink(providerID);
					setAppSupportsRevoke(supports_revocation);
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
				info={appSupportsRevoke ? TryRevokeInfo : NoRevokeInfo}
				label="Name of the application to unlink"
				isOpen={appToUnlink !== undefined}
				confirmLoading={unlinkAppMutation.isPending}
				name={appToUnlink ?? ""}
				entity="application"
				onCancel={() => setAppToUnlink(undefined)}
				onConfirm={async () => {
					try {
						const unlinkResp = await unlinkAppMutation.mutateAsync(
							appToUnlink!,
						);
						// setAppToUnlink closes the modal
						setAppToUnlink(undefined);
						// refetch repopulates the external auth data
						await externalAuthsQuery.refetch();
						// this tells our child components to refetch their data
						// as at least 1 provider was unlinked.
						setUnlinked(unlinked + 1);
						displaySuccess(
							unlinkResp.token_revoked ? RevokeSuccess : RevokeFailed,
						);
						if (
							unlinkResp.token_revocation_error &&
							unlinkResp.token_revocation_error.length > 0
						) {
							displayError(
								`Failed to revoke token from the OAuth2 provider: ${unlinkResp.token_revocation_error}`,
							);
						}
					} catch (e) {
						displayError(getErrorMessage(e, "Error unlinking application."));
					}
				}}
			/>
		</Section>
	);
};

export default ExternalAuthPage;
