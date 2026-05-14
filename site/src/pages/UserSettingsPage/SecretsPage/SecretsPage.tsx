import { type FC, useEffect, useEffectEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { watchUserSecrets } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createUserSecret,
	deleteUserSecret,
	updateUserSecret,
	userSecrets,
} from "#/api/queries/userSecrets";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import { SecretsPageView } from "./SecretsPageView";

const SecretsPage: FC = () => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const secretsQueryOptions = userSecrets(me.id);
	const secretsQuery = useQuery(secretsQueryOptions);
	const createSecretMutation = useMutation(
		createUserSecret(queryClient, me.id),
	);
	const updateSecretMutation = useMutation(
		updateUserSecret(queryClient, me.id),
	);
	const deleteSecretMutation = useMutation(
		deleteUserSecret(queryClient, me.id),
	);

	const invalidateSecrets = useEffectEvent(() => {
		void queryClient.invalidateQueries({
			queryKey: secretsQueryOptions.queryKey,
		});
	});

	useEffect(() => {
		return createReconnectingWebSocket({
			connect: () => {
				const socket = watchUserSecrets(me.id);
				socket.addEventListener("message", (event) => {
					if (event.parseError) {
						toast.error("Unable to process latest secrets update.", {
							description: "Please try refreshing the browser.",
						});
						return;
					}

					if (event.parsedMessage.user_id !== me.id) {
						return;
					}
					invalidateSecrets();
				});
				return socket;
			},
			onOpen: () => invalidateSecrets(),
		});
	}, [me.id]);

	return (
		<SecretsPageView
			secrets={secretsQuery.data}
			isLoading={!secretsQuery.isFetched && secretsQuery.isFetching}
			hasLoaded={secretsQuery.isSuccess}
			isRefreshing={secretsQuery.isFetching && secretsQuery.isFetched}
			isCreating={createSecretMutation.isPending}
			isUpdating={updateSecretMutation.isPending}
			isDeleting={deleteSecretMutation.isPending}
			getSecretsError={secretsQuery.error}
			onRefresh={() => {
				void queryClient.invalidateQueries({
					queryKey: secretsQueryOptions.queryKey,
				});
			}}
			onCreateSecret={async (request) => {
				const secret = await createSecretMutation.mutateAsync(request);
				toast.success(`Created secret "${secret.name}" successfully.`);
				return secret;
			}}
			onUpdateSecret={async (name, request) => {
				const secret = await updateSecretMutation.mutateAsync({
					name,
					request,
				});
				toast.success(`Updated secret "${secret.name}" successfully.`);
				return secret;
			}}
			onDeleteSecret={async (secret) => {
				try {
					await deleteSecretMutation.mutateAsync(secret.name);
					toast.success(`Deleted secret "${secret.name}" successfully.`);
				} catch (error) {
					toast.error(getErrorMessage(error, "Failed to delete secret."), {
						description: getErrorDetail(error),
					});
					throw error;
				}
			}}
		/>
	);
};

export default SecretsPage;
