import isChromatic from "chromatic/isChromatic";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { CliInstallPageView } from "./CliInstallPageView";

export const CliInstallPage: FC = () => {
	const origin = isChromatic() ? "https://example.com" : window.location.origin;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Install the Coder CLI")}</title>
			</Helmet>
			<CliInstallPageView origin={origin} />
		</>
	);
};

export default CliInstallPage;
