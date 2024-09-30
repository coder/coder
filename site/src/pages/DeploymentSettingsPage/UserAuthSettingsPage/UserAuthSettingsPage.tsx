import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const UserAuthSettingsPage: FC = () => {
	const { deploymentValues } = useManagementSettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle("User Authentication Settings")}</title>
			</Helmet>

			{deploymentValues ? (
				<UserAuthSettingsPageView options={deploymentValues.options} />
			) : (
				<Loader />
			)}
		</>
	);
};

export default UserAuthSettingsPage;
