import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { CliInstallPageView } from "./CliInstallPageView";

export const CliInstallPage: FC = () => {
	return (
		<>
			<Helmet>
				<title>{pageTitle("Install the Coder CLI")}</title>
			</Helmet>
			<CliInstallPageView />
		</>
	);
};

export default CliInstallPage;
