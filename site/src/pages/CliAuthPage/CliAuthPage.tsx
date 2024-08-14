import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { apiKey } from "api/queries/users";
import { pageTitle } from "utils/page";
import { CliAuthPageView } from "./CliAuthPageView";

export const CliAuthenticationPage: FC = () => {
  const { data } = useQuery(apiKey());

  return (
    <>
      <Helmet>
        <title>{pageTitle("CLI Auth")}</title>
      </Helmet>
      <CliAuthPageView sessionToken={data?.key} />
    </>
  );
};

export default CliAuthenticationPage;
