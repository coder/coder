import { createOrganization } from "api/queries/organizations";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
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
						await createOrganizationMutation.mutateAsync(values);
						displaySuccess("Organization created.");
						navigate(`/organizations/${values.name}`);
					}}
				/>
			</RequirePermission>
		</main>
	);
};

export default CreateOrganizationPage;
