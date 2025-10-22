import isChromatic from "chromatic/isChromatic";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { CliInstallPageView } from "./CliInstallPageView";

const CliInstallPage: FC = () => {
	const origin = isChromatic() ? "https://example.com" : window.location.origin;

	return (
		<>
			<title>{pageTitle("Install the Coder CLI")}</title>
			<CliInstallPageView origin={origin} />
		</>
	);
};

export default CliInstallPage;
