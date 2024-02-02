import { type FC, Suspense } from "react";
import { Outlet } from "react-router-dom";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useMe } from "contexts/auth/useMe";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
  const me = useMe();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings")}</title>
      </Helmet>

      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          <Sidebar user={me} />
          <Suspense fallback={<Loader />}>
            <main css={{ maxWidth: 800, width: "100%" }}>
              <Outlet />
            </main>
          </Suspense>
        </Stack>
      </Margins>
    </>
  );
};

export default Layout;
