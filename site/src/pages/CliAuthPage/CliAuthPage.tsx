import { apiKey } from "api/queries/users";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { CliAuthPageView } from "./CliAuthPageView";

const CliAuthenticationPage: FC = () => {
	const { data } = useQuery(apiKey());

	return (
		<>
			<title>{pageTitle("CLI Auth")}</title>
			<CliAuthPageView sessionToken={data?.key} />
		</>
	);
};

export default CliAuthenticationPage;
