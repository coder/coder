import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import Button from "@mui/material/Button";
import type { FC } from "react";
import { Link } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";
import { Header } from "../Header";
import { OAuth2AppForm } from "./OAuth2AppForm";

type CreateOAuth2AppProps = {
  isUpdating: boolean;
  createApp: (req: TypesGen.PostOAuth2ProviderAppRequest) => void;
  error?: unknown;
};

export const CreateOAuth2AppPageView: FC<CreateOAuth2AppProps> = ({
  isUpdating,
  createApp,
  error,
}) => {
  return (
    <>
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <Header
          title="Add an OAuth2 application"
          description="Configure an application to use Coder as an OAuth2 provider."
        />
        <Button
          component={Link}
          startIcon={<KeyboardArrowLeft />}
          to="/deployment/oauth2-provider/apps"
        >
          All OAuth2 Applications
        </Button>
      </Stack>

      <Stack>
        {error ? <ErrorAlert error={error} /> : undefined}
        <OAuth2AppForm
          onSubmit={createApp}
          isUpdating={isUpdating}
          error={error}
        />
      </Stack>
    </>
  );
};
