import { type FC, useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createUserSecret,
	deleteUserSecret,
	updateUserSecret,
	userSecrets,
} from "#/api/queries/userSecrets";
import type {
	CreateUserSecretRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
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

	const onMutationError = useCallback(
		(error: unknown, defaultMessage: string) => {
			toast.error(getErrorMessage(error, defaultMessage), {
				description: getErrorDetail(error),
			});
		},
		[],
	);

	const onCreateSecret = useCallback(
		(request: CreateUserSecretRequest) => {
			return new Promise<UserSecret>((resolve, reject) => {
				createSecretMutation.mutate(request, {
					onError: (error) => {
						onMutationError(error, "Failed to create secret.");
						reject(error);
					},
					onSuccess: (secret) => {
						toast.success("Secret created successfully.");
						resolve(secret);
					},
				});
			});
		},
		[createSecretMutation, onMutationError],
	);

	const onUpdateSecret = useCallback(
		(name: string, request: UpdateUserSecretRequest) => {
			return new Promise<UserSecret>((resolve, reject) => {
				updateSecretMutation.mutate(
					{ name, request },
					{
						onError: (error) => {
							onMutationError(error, "Failed to update secret.");
							reject(error);
						},
						onSuccess: (secret) => {
							toast.success("Secret updated successfully.");
							resolve(secret);
						},
					},
				);
			});
		},
		[onMutationError, updateSecretMutation],
	);

	const onDeleteSecret = useCallback(
		(secret: UserSecret) => {
			deleteSecretMutation.mutate(secret.name, {
				onError: (error) => {
					onMutationError(error, "Failed to delete secret.");
				},
				onSuccess: () => {
					toast.success("Secret deleted successfully.");
				},
			});
		},
		[deleteSecretMutation, onMutationError],
	);

	return (
		<SecretsPageView
			secrets={secretsQuery.data}
			isLoading={!secretsQuery.isFetched && secretsQuery.isFetching}
			hasLoaded={secretsQuery.isFetched}
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
			onCreateSecret={onCreateSecret}
			onUpdateSecret={onUpdateSecret}
			onDeleteSecret={onDeleteSecret}
		/>
	);
};

export default SecretsPage;
