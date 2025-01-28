import { createOrganization } from "api/queries/organizations";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const CreateOrganizationPage: FC = () => {
	const navigate = useNavigate();
	const feats = useFeatureVisibility();

	const queryClient = useQueryClient();
	const createOrganizationMutation = useMutation(
		createOrganization(queryClient),
	);

	const error = createOrganizationMutation.error;

	return (
		<main className="py-7">
			<CreateOrganizationPageView
				error={error}
				isEntitled={feats.multiple_organizations}
				onSubmit={async (values) => {
					await createOrganizationMutation.mutateAsync(values);
					displaySuccess("Organization created.");
					navigate(`/organizations/${values.name}`);
				}}
			/>
		</main>
	);
};

export default CreateOrganizationPage;
