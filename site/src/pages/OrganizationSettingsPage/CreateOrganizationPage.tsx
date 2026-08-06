import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const CreateOrganizationPage: FC = () => {
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	return (
		<RequirePermission isFeatureVisible={permissions.createOrganization}>
			<title>{pageTitle("New Organization")}</title>
			<CreateOrganizationPageView isEntitled={feats.multiple_organizations} />
		</RequirePermission>
	);
};

export default CreateOrganizationPage;
