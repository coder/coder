import { css } from "@emotion/css";
import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { Stack } from "components/Stack/Stack";
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { FormikContextType, useFormik } from "formik";
import { useEffect, useState, type FC } from "react";
import { useSearchParams } from "react-router-dom";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";
import * as Yup from "yup";
import {
  CreateWorkspaceMode,
  type ExternalAuthPollingState,
} from "./CreateWorkspacePage";
import { ExternalAuth } from "./ExternalAuth";
import {
  ProvisionerGroupSelect,
  useProvisionerDaemonGroups,
} from "./ProvisionerDaemons";
import { SelectedTemplate } from "./SelectedTemplate";
import { CreateWSPermissions } from "./permissions";

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
  provisionerDaemons: TypesGen.ProvisionerDaemon[];
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
  provisionerDaemons,
  permissions,
  creatingWorkspace,
  onSubmit,
  onCancel,
}) => {
  const theme = useTheme();
  const [owner, setOwner] = useState(defaultOwner);
  const { verifyExternalAuth, externalAuthErrors } =
    useExternalAuthVerification(externalAuth);
  const [searchParams] = useSearchParams();
  const disabledParamsList = searchParams?.get("disable_params")?.split(",");
  const provisionerDaemonGroups =
    useProvisionerDaemonGroups(provisionerDaemons);

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: defaultName,
        template_id: template.id,
        rich_parameter_values: getInitialRichParameterValues(
          parameters,
          defaultBuildParameters,
        ),
        provisioner_tags: {},
      },
      validationSchema: Yup.object({
        name: nameValidator("Workspace Name"),
        rich_parameter_values: useValidationSchemaForRichParameters(parameters),
      }),
      enableReinitialize: true,
      onSubmit: (request) => {
        if (!verifyExternalAuth()) {
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

        {provisionerDaemonGroups.length > 1 && (
          <FormSection
            title="Provision Destination"
            description="Select a destination for your workspace to be provisioned. This cannot be changed after creation."
          >
            <FormFields>
              <ProvisionerGroupSelect
                groups={provisionerDaemonGroups}
                onSelectGroup={async (group) => {
                  await form.setFieldValue("provisioner_tags", group[0].tags);
                }}
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
              {externalAuth.map((auth) => (
                <ExternalAuth
                  key={auth.id}
                  authenticateURL={auth.authenticate_url}
                  authenticated={auth.authenticated}
                  externalAuthPollingState={externalAuthPollingState}
                  startPollingExternalAuth={startPollingExternalAuth}
                  displayName={auth.display_name}
                  displayIcon={auth.display_icon}
                  error={externalAuthErrors[auth.id]}
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

type ExternalAuthErrors = Record<string, string>;

const useExternalAuthVerification = (
  externalAuth: TypesGen.TemplateVersionExternalAuth[],
) => {
  const [externalAuthErrors, setExternalAuthErrors] =
    useState<ExternalAuthErrors>({});

  // Clear errors when externalAuth is refreshed
  useEffect(() => {
    setExternalAuthErrors({});
  }, [externalAuth]);

  const verifyExternalAuth = () => {
    const errors: ExternalAuthErrors = {};

    for (let i = 0; i < externalAuth.length; i++) {
      const auth = externalAuth.at(i);
      if (!auth) {
        continue;
      }
      if (!auth.authenticated) {
        errors[auth.id] = "You must authenticate to create a workspace!";
      }
    }

    setExternalAuthErrors(errors);
    const isValid = Object.keys(errors).length === 0;
    return isValid;
  };

  return {
    externalAuthErrors,
    verifyExternalAuth,
  };
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
