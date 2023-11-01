import { type FC, Suspense } from "react";
import { Outlet } from "react-router-dom";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useMe } from "hooks/useMe";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { Margins } from "../Margins/Margins";
import { Sidebar } from "./Sidebar";

export const SettingsLayout: FC = () => {
  const me = useMe();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings")}</title>
      </Helmet>

      <Margins>
        <Stack
          css={(theme) => ({
            padding: theme.spacing(6, 0),
          })}
          direction="row"
          spacing={6}
        >
          <Sidebar user={me} />
          <Suspense fallback={<Loader />}>
            <main
              css={{
                maxWidth: 800,
                width: "100%",
              }}
            >
              <Outlet />
            </main>
          </Suspense>
        </Stack>
      </Margins>
    </>
  );
};
