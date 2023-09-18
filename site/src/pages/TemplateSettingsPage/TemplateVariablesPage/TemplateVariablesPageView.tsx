import {
  CreateTemplateVersionRequest,
  TemplateVersion,
  TemplateVersionVariable,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ComponentProps, FC } from "react";
import { TemplateVariablesForm } from "./TemplateVariablesForm";
import { makeStyles } from "@mui/styles";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";

export interface TemplateVariablesPageViewProps {
  templateVersion?: TemplateVersion;
  templateVariables?: TemplateVersionVariable[];
  onSubmit: (data: CreateTemplateVersionRequest) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  errors?: {
    /**
     * Failed to create a new template version
     */
    createVersionError?: unknown;
    /**
     * Failed to poll job status of new template version
     */
    statusError?: unknown;
    /**
     * Job for new template version failed
     */
    jobError?: TemplateVersion["job"]["error"];
    /**
     * New version was created successfully, but publishing it failed
     */
    publishError?: unknown;
  };
  initialTouched?: ComponentProps<
    typeof TemplateVariablesForm
  >["initialTouched"];
}

export const TemplateVariablesPageView: FC<TemplateVariablesPageViewProps> = ({
  templateVersion,
  templateVariables,
  onCancel,
  onSubmit,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const classes = useStyles();
  const hasError = Object.values(errors).some((error) => Boolean(error));

  console.log("LOOK AT ME", errors);

  return (
    <>
      <PageHeader className={classes.pageHeader}>
        <PageHeaderTitle>Template variables</PageHeaderTitle>
      </PageHeader>
      {hasError && (
        <Stack className={classes.errorContainer}>
          {Boolean(errors.createVersionError) && (
            <ErrorAlert error={errors.createVersionError} />
          )}
          {Boolean(errors.statusError) && (
            <ErrorAlert error={errors.statusError} />
          )}
          {Boolean(errors.jobError) && <ErrorAlert error={errors.jobError} />}
          {Boolean(errors.publishError) && (
            <ErrorAlert error={errors.publishError} />
          )}
        </Stack>
      )}
      {templateVersion && templateVariables && templateVariables.length > 0 && (
        <TemplateVariablesForm
          initialTouched={initialTouched}
          isSubmitting={isSubmitting}
          templateVersion={templateVersion}
          templateVariables={templateVariables}
          onSubmit={onSubmit}
          onCancel={onCancel}
          error={errors.statusError}
        />
      )}
      {templateVariables && templateVariables.length === 0 && (
        <Alert severity="info">
          This template does not use managed variables.
        </Alert>
      )}
    </>
  );
};

const useStyles = makeStyles((theme) => ({
  errorContainer: {
    marginBottom: theme.spacing(8),
  },
  goBackSection: {
    display: "flex",
    width: "100%",
    marginTop: 32,
  },
  pageHeader: {
    paddingTop: 0,
  },
}));
