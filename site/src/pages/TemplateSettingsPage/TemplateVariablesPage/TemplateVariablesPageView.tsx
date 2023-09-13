import {
  CreateTemplateVersionRequest,
  TemplateVersion,
  TemplateVersionVariable,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Loader } from "components/Loader/Loader";
import { ComponentProps, FC } from "react";
import { TemplateVariablesForm } from "./TemplateVariablesForm";
import { makeStyles } from "@mui/styles";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export interface TemplateVariablesPageViewProps {
  templateVersion?: TemplateVersion;
  templateVariables?: TemplateVersionVariable[];
  onSubmit: (data: CreateTemplateVersionRequest) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  errors?: {
    getTemplateDataError?: unknown;
    updateTemplateError?: unknown;
    jobError?: TemplateVersion["job"]["error"];
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
  const isLoading =
    !templateVersion &&
    !templateVariables &&
    !errors.getTemplateDataError &&
    !errors.updateTemplateError;
  const hasError = Object.values(errors).some((error) => Boolean(error));
  return (
    <>
      <PageHeader className={classes.pageHeader}>
        <PageHeaderTitle>Template variables</PageHeaderTitle>
      </PageHeader>
      {hasError && (
        <div className={classes.errorContainer}>
          {Boolean(errors.getTemplateDataError) && (
            <ErrorAlert error={errors.getTemplateDataError} />
          )}
          {Boolean(errors.updateTemplateError) && (
            <ErrorAlert error={errors.updateTemplateError} />
          )}
          {Boolean(errors.jobError) && <ErrorAlert error={errors.jobError} />}
        </div>
      )}
      {isLoading && <Loader />}
      {templateVersion && templateVariables && templateVariables.length > 0 && (
        <TemplateVariablesForm
          initialTouched={initialTouched}
          isSubmitting={isSubmitting}
          templateVersion={templateVersion}
          templateVariables={templateVariables}
          onSubmit={onSubmit}
          onCancel={onCancel}
          error={errors.updateTemplateError}
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
