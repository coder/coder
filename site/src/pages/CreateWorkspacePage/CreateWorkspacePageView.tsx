import { css } from "@emotion/css";
import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import type * as TypesGen from "api/typesGenerated";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { FormikContextType, useFormik } from "formik";
import { type FC, useEffect, useState } from "react";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { SelectedTemplate } from "./SelectedTemplate";
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/Form/Form";
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters";
import { ExternalAuthButton } from "./ExternalAuthButton";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";
import {
  CreateWorkspaceMode,
  ExternalAuthPollingState,
} from "./CreateWorkspacePage";
import { useSearchParams } from "react-router-dom";
import { CreateWSPermissions } from "./permissions";
import { Alert } from "components/Alert/Alert";

export const Language = {
  duplicationWarning:
    "Duplicating a workspace only copies its parameters. No state from the old workspace is copied over.",
} as const;

export interface CreateWorkspacePageViewProps {
  mode: CreateWorkspaceMode;
  error: unknown;
  resetMutation: () => void;
  defaultName: string;
  defaultOwner: TypesGen.User;
  template: TypesGen.Template;
  versionId?: string;
  externalAuth: TypesGen.TemplateVersionExternalAuth[];
  externalAuthPollingState: ExternalAuthPollingState;
  startPollingExternalAuth: () => void;
  parameters: TypesGen.TemplateVersionParameter[];
  defaultBuildParameters: TypesGen.WorkspaceBuildParameter[];
  permissions: CreateWSPermissions;
  creatingWorkspace: boolean;
  onCancel: () => void;
  onSubmit: (
    req: TypesGen.CreateWorkspaceRequest,
    owner: TypesGen.User,
  ) => void;
}

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = ({
  mode,
  error,
  resetMutation,
  defaultName,
  defaultOwner,
  template,
  versionId,
  externalAuth,
  externalAuthPollingState,
  startPollingExternalAuth,
  parameters,
  defaultBuildParameters,
  permissions,
  creatingWorkspace,
  onSubmit,
  onCancel,
}) => {
  const theme = useTheme();
  const [owner, setOwner] = useState(defaultOwner);
  const [searchParams] = useSearchParams();
  const disabledParamsList = searchParams?.get("disable_params")?.split(",");
  const requiresExternalAuth = externalAuth.some((auth) => !auth.authenticated);

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: defaultName,
        template_id: template.id,
        rich_parameter_values: getInitialRichParameterValues(
          parameters,
          defaultBuildParameters,
        ),
      },
      validationSchema: Yup.object({
        name: nameValidator("Workspace Name"),
        rich_parameter_values: useValidationSchemaForRichParameters(parameters),
      }),
      enableReinitialize: true,
      onSubmit: (request) => {
        if (requiresExternalAuth) {
          return;
        }

        onSubmit(request, owner);
      },
    });

  useEffect(() => {
    if (error) {
      window.scrollTo(0, 0);
    }
  }, [error]);

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    error,
  );

  return (
    <FullPageHorizontalForm title="New workspace" onCancel={onCancel}>
      <HorizontalForm onSubmit={form.handleSubmit}>
        {Boolean(error) && <ErrorAlert error={error} />}

        {mode === "duplicate" && (
          <Alert severity="info" dismissible>
            {Language.duplicationWarning}
          </Alert>
        )}

        {/* General info */}
        <FormSection
          title="General"
          description="The template and name of your new workspace."
        >
          <FormFields>
            <SelectedTemplate template={template} />
            {versionId && versionId !== template.active_version_id && (
              <Stack spacing={1} css={styles.hasDescription}>
                <TextField
                  disabled
                  fullWidth
                  value={versionId}
                  label="Version ID"
                />
                <span css={styles.description}>
                  This parameter has been preset, and cannot be modified.
                </span>
              </Stack>
            )}
            <TextField
              {...getFieldHelpers("name")}
              disabled={creatingWorkspace}
              // resetMutation facilitates the clearing of validation errors
              onChange={onChangeTrimmed(form, resetMutation)}
              autoFocus
              fullWidth
              label="Workspace Name"
            />
          </FormFields>
        </FormSection>

        {permissions.createWorkspaceForUser && (
          <FormSection
            title="Workspace Owner"
            description="Only admins can create workspace for other users."
          >
            <FormFields>
              <UserAutocomplete
                value={owner}
                onChange={(user) => {
                  setOwner(user ?? defaultOwner);
                }}
                label="Owner"
                size="medium"
              />
            </FormFields>
          </FormSection>
        )}

        {externalAuth && externalAuth.length > 0 && (
          <FormSection
            title="External Authentication"
            description="This template requires authentication to external services."
          >
            <FormFields>
              {requiresExternalAuth && (
                <Alert severity="error">
                  To create a workspace using the selected template, please
                  ensure you are authenticated with all the external providers
                  listed below.
                </Alert>
              )}
              {externalAuth.map((auth) => (
                <ExternalAuthButton
                  key={auth.id}
                  auth={auth}
                  isLoading={externalAuthPollingState === "polling"}
                  onStartPolling={startPollingExternalAuth}
                  displayRetry={externalAuthPollingState === "abandoned"}
                />
              ))}
            </FormFields>
          </FormSection>
        )}

        {parameters && (
          <>
            <MutableTemplateParametersSection
              templateParameters={parameters}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
                  },
                  disabled:
                    disabledParamsList?.includes(
                      parameter.name.toLowerCase().replace(/ /g, "_"),
                    ) || creatingWorkspace,
                };
              }}
            />
            <ImmutableTemplateParametersSection
              templateParameters={parameters}
              classes={{
                root: css`
                  border: 1px solid ${theme.palette.warning.light};
                  border-radius: 8px;
                  background-color: ${theme.palette.background.paper};
                  padding: 80px;
                  margin-left: -80px;
                  margin-right: -80px;
                `,
              }}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
                  },
                  disabled:
                    disabledParamsList?.includes(
                      parameter.name.toLowerCase().replace(/ /g, "_"),
                    ) || creatingWorkspace,
                };
              }}
            />
          </>
        )}

        <FormFooter
          onCancel={onCancel}
          isLoading={creatingWorkspace}
          submitLabel="Create Workspace"
        />
      </HorizontalForm>
    </FullPageHorizontalForm>
  );
};

const styles = {
  hasDescription: {
    paddingBottom: 16,
  },
  description: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
