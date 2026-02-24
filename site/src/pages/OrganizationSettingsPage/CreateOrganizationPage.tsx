import { createOrganization } from "api/queries/organizations";
import { useAuthenticated } from "hooks";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const CreateOrganizationPage: FC = () => {
	const navigate = useNavigate();
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	const queryClient = useQueryClient();
	const createOrganizationMutation = useMutation(
		createOrganization(queryClient),
	);

	const error = createOrganizationMutation.error;

	return (
		<main className="py-7">
			<RequirePermission isFeatureVisible={permissions.createOrganization}>
				<CreateOrganizationPageView
					error={error}
					isEntitled={feats.multiple_organizations}
					onSubmit={async (values) => {
						await createOrganizationMutation.mutateAsync(values, {
							onSuccess: () => {
								toast.success(
									`Organization "${values.name}" created successfully.`,
								);
								navigate(`/organizations/${values.name}`);
							},
						});
					}}
				/>
			</RequirePermission>
		</main>
	);
};

export default CreateOrganizationPage;
