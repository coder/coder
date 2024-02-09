import { useState, type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { DuplicateTemplateView } from "./DuplicateTemplateView";
import { ImportStarterTemplateView } from "./ImportStarterTemplateView";
import { UploadTemplateView } from "./UploadTemplateView";
import { BuildLogsDrawer } from "./BuildLogsDrawer";
import { useMutation } from "react-query";
import { createTemplate } from "api/queries/templates";
import { CreateTemplatePageViewProps } from "./types";
import { TemplateVersion } from "api/typesGenerated";

const CreateTemplatePage: FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [isBuildLogsOpen, setIsBuildLogsOpen] = useState(false);
  const [templateVersion, setTemplateVersion] = useState<TemplateVersion>();
  const createTemplateMutation = useMutation(createTemplate());

  const onCancel = () => {
    navigate(-1);
  };

  const pageViewProps: CreateTemplatePageViewProps = {
    onCreateTemplate: async (options) => {
      setIsBuildLogsOpen(true);
      const template = await createTemplateMutation.mutateAsync({
        ...options,
        onCreateVersion: setTemplateVersion,
      });
      navigate(`/templates/${template.name}/files`);
    },
    error: createTemplateMutation.error,
    isCreating: createTemplateMutation.isLoading,
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Template")}</title>
      </Helmet>

      <FullPageHorizontalForm title="Create Template" onCancel={onCancel}>
        {searchParams.has("fromTemplate") ? (
          <DuplicateTemplateView {...pageViewProps} />
        ) : searchParams.has("exampleId") ? (
          <ImportStarterTemplateView {...pageViewProps} />
        ) : (
          <UploadTemplateView {...pageViewProps} />
        )}
      </FullPageHorizontalForm>

      <BuildLogsDrawer
        open={isBuildLogsOpen}
        onClose={() => setIsBuildLogsOpen(false)}
        templateVersion={templateVersion}
      />
    </>
  );
};

export default CreateTemplatePage;
