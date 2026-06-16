import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	aiGatewayKeysList,
	createAIGatewayKeyMutation,
	deleteAIGatewayKeyMutation,
} from "#/api/queries/aiGatewayKeys";
import type { AIGatewayKey } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { CreateGatewayKeyDialog } from "./CreateGatewayKeyDialog";
import { GatewayKeysPageView } from "./GatewayKeysPageView";

const GatewayKeysPage: FC = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
	const showPaywall = !featureVisibility.aibridge;

	const queryClient = useQueryClient();
	const keysQuery = useQuery({
		...aiGatewayKeysList(),
		enabled: !showPaywall,
	});
	const createMutation = useMutation(createAIGatewayKeyMutation(queryClient));
	const deleteMutation = useMutation(deleteAIGatewayKeyMutation(queryClient));

	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [keyToDelete, setKeyToDelete] = useState<AIGatewayKey | undefined>(
		undefined,
	);

	return (
		<RequirePermission isFeatureVisible={permissions.viewAIGatewayKeys}>
			<title>{pageTitle("AI Gateway Keys")}</title>

			<GatewayKeysPageView
				keys={keysQuery.data ?? []}
				isLoading={keysQuery.isLoading}
				error={keysQuery.error}
				showPaywall={showPaywall}
				onCreateKey={() => setIsCreateOpen(true)}
				onDeleteKey={setKeyToDelete}
			/>

			<CreateGatewayKeyDialog
				open={isCreateOpen}
				onClose={() => {
					createMutation.reset();
					setIsCreateOpen(false);
				}}
				onCreate={(name) => createMutation.mutate({ name })}
				createdKey={createMutation.data}
				submitError={createMutation.error}
				isSubmitting={createMutation.isPending}
			/>

			<ConfirmDialog
				type="delete"
				title="Delete AI Gateway key"
				description={
					<>
						Are you sure you want to permanently delete key{" "}
						<strong>{keyToDelete?.name}</strong>? Any AI Gateway replica using
						it will no longer be able to authenticate.
					</>
				}
				open={Boolean(keyToDelete)}
				confirmLoading={deleteMutation.isPending}
				onConfirm={() => {
					if (!keyToDelete) {
						return;
					}
					const name = keyToDelete.name;
					deleteMutation.mutate(keyToDelete.id, {
						onSuccess: () => {
							toast.success(`Deleted AI Gateway key "${name}" successfully.`);
							setKeyToDelete(undefined);
						},
						onError: (error) => {
							toast.error(
								getErrorMessage(error, "Failed to delete AI Gateway key."),
							);
						},
					});
				}}
				onClose={() => setKeyToDelete(undefined)}
			/>
		</RequirePermission>
	);
};

export default GatewayKeysPage;
