import { useEffect, useState, FC, PropsWithChildren } from "react";
import { Helmet } from "react-helmet-async";
import { getApiKey } from "../../api/api";
import { pageTitle } from "../../utils/page";
import { CliAuthPageView } from "./CliAuthPageView";

export const CliAuthenticationPage: FC<PropsWithChildren<unknown>> = () => {
  const [apiKey, setApiKey] = useState<string | null>(null);

  useEffect(() => {
    getApiKey()
      .then(({ key }) => {
        setApiKey(key);
      })
      .catch((error) => {
        console.error(error);
      });
  }, []);

  return (
    <>
      <Helmet>
        <title>{pageTitle("CLI Auth")}</title>
      </Helmet>
      <CliAuthPageView sessionToken={apiKey} />
    </>
  );
};

export default CliAuthenticationPage;
