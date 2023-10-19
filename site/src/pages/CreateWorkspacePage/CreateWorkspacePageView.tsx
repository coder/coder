import TextField from "@mui/material/TextField";
import * as TypesGen from "api/typesGenerated";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { FormikContextType, useFormik } from "formik";
import { type FC, useEffect, useState, useReducer } from "react";
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
import { makeStyles } from "@mui/styles";
import {
  getInitialRichParameterValues,
  validateRichParameters,
} from "utils/richParameters";
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters";
import { ExternalAuth } from "./ExternalAuth";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";
import { useSearchParams } from "react-router-dom";
import { CreateWSPermissions } from "./permissions";
import { useTheme } from "@emotion/react";
import { Alert } from "components/Alert/Alert";
import { Margins } from "components/Margins/Margins";
import {
  type ExternalAuthPollingStatus,
  type CreateWorkspaceMode,
} from "./CreateWorkspacePage";

export interface CreateWorkspacePageViewProps {
  error: unknown;
  defaultName: string;
  defaultOwner: TypesGen.User;
  template: TypesGen.Template;
  versionId?: string;
  externalAuth: TypesGen.TemplateVersionExternalAuth[];
  externalAuthPollingStatus: ExternalAuthPollingStatus;
  startPollingExternalAuth: () => void;
  parameters: TypesGen.TemplateVersionParameter[];
  defaultBuildParameters: TypesGen.WorkspaceBuildParameter[];
  permissions: CreateWSPermissions;
  creatingWorkspace: boolean;
  onCancel: () => void;
  mode: CreateWorkspaceMode;
  onSubmit: (
    req: TypesGen.CreateWorkspaceRequest,
    owner: TypesGen.User,
  ) => void;
}

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = ({
  error,
  defaultName,
  defaultOwner,
  template,
  versionId,
  externalAuth,
  externalAuthPollingStatus,
  startPollingExternalAuth,
  parameters,
  defaultBuildParameters,
  permissions,
  creatingWorkspace,
  onSubmit,
  onCancel,
  mode,
}) => {
  const styles = useStyles();
  const [owner, setOwner] = useState(defaultOwner);
  const [searchParams] = useSearchParams();
  const disabledParamsList = searchParams?.get("disable_params")?.split(",");

  const authErrors = getAuthErrors(externalAuth);
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
        rich_parameter_values: validateRichParameters(parameters),
      }),
      enableReinitialize: true,
      onSubmit: (request) => {
        const errorCount = Object.keys(authErrors).length;
        if (errorCount > 0) {
          form.setSubmitting(false);
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
    <>
      {mode === "duplicate" && <DuplicateWarningMessage />}

      <FullPageHorizontalForm title="New workspace" onCancel={onCancel}>
        <HorizontalForm onSubmit={form.handleSubmit}>
          {Boolean(error) && <ErrorAlert error={error} />}

          {/* General info */}
          <FormSection
            title="General"
            description="The template and name of your new workspace."
          >
            <FormFields>
              <SelectedTemplate template={template} />
              {versionId && versionId !== template.active_version_id && (
                <Stack spacing={1} className={styles.hasDescription}>
                  <TextField
                    disabled
                    fullWidth
                    value={versionId}
                    label="Version ID"
                  />
                  <span className={styles.description}>
                    This parameter has been preset, and cannot be modified.
                  </span>
                </Stack>
              )}
              <TextField
                {...getFieldHelpers("name")}
                disabled={creatingWorkspace}
                onChange={onChangeTrimmed(form)}
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
                {externalAuth.map((auth) => (
                  <ExternalAuth
                    key={auth.id}
                    authenticateURL={auth.authenticate_url}
                    authenticated={auth.authenticated}
                    externalAuthPollingState={externalAuthPollingStatus}
                    startPollingExternalAuth={startPollingExternalAuth}
                    displayName={auth.display_name}
                    displayIcon={auth.display_icon}
                    error={authErrors[auth.id]}
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
                      await form.setFieldValue(
                        "rich_parameter_values." + index,
                        {
                          name: parameter.name,
                          value: value,
                        },
                      );
                    },
                    disabled:
                      disabledParamsList?.includes(
                        parameter.name.toLowerCase().replace(/ /g, "_"),
                      ) || form.isSubmitting,
                  };
                }}
              />

              <ImmutableTemplateParametersSection
                templateParameters={parameters}
                classes={{ root: styles.warningSection }}
                getInputProps={(parameter, index) => {
                  return {
                    ...getFieldHelpers(
                      "rich_parameter_values[" + index + "].value",
                    ),
                    onChange: async (value) => {
                      await form.setFieldValue(
                        "rich_parameter_values." + index,
                        {
                          name: parameter.name,
                          value: value,
                        },
                      );
                    },
                    disabled:
                      disabledParamsList?.includes(
                        parameter.name.toLowerCase().replace(/ /g, "_"),
                      ) || form.isSubmitting,
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
    </>
  );
};

function getAuthErrors(
  authList: readonly TypesGen.TemplateVersionExternalAuth[],
): Readonly<Record<string, string>> {
  const authErrors: Record<string, string> = {};

  for (const auth of authList) {
    if (!auth.authenticated) {
      authErrors[auth.id] = "You must authenticate to create a workspace!";
    }
  }

  return authErrors;
}

function DuplicateWarningMessage() {
  const [isDismissed, dismiss] = useReducer(() => true, false);
  const theme = useTheme();

  if (isDismissed) {
    return null;
  }

  // Setup looks a little hokey (having an Alert already fully configured to
  // listen to dismissals, on top of more dismissal state), but relying solely
  // on the Alert API wouldn't get rid of the div and horizontal margin helper
  // after the dismiss happens. Not using CSS margins because those can be a
  // style maintenance nightmare over time
  return (
    <div css={{ paddingTop: theme.spacing(6) }}>
      <Margins size="medium">
        <Alert severity="warning" dismissible onDismiss={dismiss}>
          Duplicating a workspace only copies its parameters. No state from the
          old workspace is copied over.
        </Alert>
      </Margins>
    </div>
  );
}

const useStyles = makeStyles((theme) => ({
  hasDescription: {
    paddingBottom: theme.spacing(2),
  },
  description: {
    fontSize: 13,
    color: theme.palette.text.secondary,
  },
  warningText: {
    color: theme.palette.warning.light,
  },
  warningSection: {
    border: `1px solid ${theme.palette.warning.light}`,
    borderRadius: 8,
    backgroundColor: theme.palette.background.paper,
    padding: theme.spacing(10),
    marginLeft: theme.spacing(-10),
    marginRight: theme.spacing(-10),
  },
}));
