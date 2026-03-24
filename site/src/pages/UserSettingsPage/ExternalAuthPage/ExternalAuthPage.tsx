import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	externalAuths,
	unlinkExternalAuths,
	validateExternalAuth,
} from "api/queries/externalAuth";
import type { ExternalAuthLinkProvider } from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
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
		<Section title="External Authentication" layout="fluid">
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
							toast.success("Application link is valid.");
						} else {
							toast.error("Application link is not valid.", {
								description:
									"Please unlink the application and reauthenticate.",
							});
						}
					} catch (error) {
						toast.error(
							getErrorMessage(error, "Error validating application link."),
							{
								description: getErrorDetail(error),
							},
						);
					}
				}}
			/>
			<DeleteDialog
				key={appToUnlink?.id}
				title="Unlink Application"
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
						toast.success(
							unlinkResp.token_revoked
								? "Successfully deleted external auth link and revoked token from the OAuth2 provider."
								: "Successfully deleted external auth link. Token has NOT been revoked from the OAuth2 provider.",
						);
					} catch (e) {
						toast.error(getErrorMessage(e, "Error unlinking application."), {
							description: getErrorDetail(e),
						});
					}
				}}
			/>
		</Section>
	);
};

export default ExternalAuthPage;
