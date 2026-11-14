import { useQuery } from "react-query";
import { apiKey } from "#/api/queries/users";
import { pageTitle } from "#/utils/page";
import { CliAuthPageView } from "./CliAuthPageView";

const CliAuthenticationPage: React.FC = () => {
	const { data } = useQuery(apiKey());

	return (
		<>
			<title>{pageTitle("CLI auth")}</title>
			<CliAuthPageView sessionToken={data?.key} />
		</>
	);
};

export default CliAuthenticationPage;
