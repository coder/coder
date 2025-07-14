import { getErrorMessage } from "api/errors";
import { postApp } from "api/queries/oauth2";
import type * as TypesGen from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { ClientCredentialsAppForm } from "components/OAuth2/ClientCredentialsAppForm";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";

type ClientCredentialsFormData = {
	name: string;
	icon: string;
	grant_types: TypesGen.OAuth2ProviderGrantType[];
	redirect_uris: string[];
};

const CreateClientCredentialsAppPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createAppMutation = useMutation(postApp(queryClient));

	const handleSubmit = async (data: ClientCredentialsFormData) => {
		try {
			const app = await createAppMutation.mutateAsync({
				name: data.name,
				icon: data.icon,
				grant_types: data.grant_types,
				redirect_uris: data.redirect_uris,
			});

			displaySuccess(`OAuth2 application "${app.name}" created successfully!`);
			navigate(`/settings/oauth2-provider/${app.id}`);
		} catch (error) {
			displayError(
				getErrorMessage(error, "Failed to create OAuth2 application."),
			);
		}
	};

	return (
		<ClientCredentialsAppForm
			onSubmit={handleSubmit}
			error={createAppMutation.error}
			isUpdating={createAppMutation.isPending}
		/>
	);
};

export default CreateClientCredentialsAppPage;
