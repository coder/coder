import { isPixel } from "@coder/pixel-storybook/storyapi";
import { pageTitle } from "#/utils/page";
import { CliInstallPageView } from "./CliInstallPageView";

const CliInstallPage: React.FC = () => {
	const origin = isPixel() ? "https://example.com" : location.origin;

	return (
		<>
			<title>{pageTitle("Install the Coder CLI")}</title>
			<CliInstallPageView origin={origin} />
		</>
	);
};

export default CliInstallPage;
