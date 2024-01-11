import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { DuplicateTemplateView } from "./DuplicateTemplateView";
import { ImportStarterTemplateView } from "./ImportStarterTemplateView";
import { UploadTemplateView } from "./UploadTemplateView";

const CreateTemplatePage: FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const onCancel = () => {
    navigate(-1);
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Template")}</title>
      </Helmet>

      <FullPageHorizontalForm title="Create Template" onCancel={onCancel}>
        {searchParams.has("fromTemplate") ? (
          <DuplicateTemplateView />
        ) : searchParams.has("exampleId") ? (
          <ImportStarterTemplateView />
        ) : (
          <UploadTemplateView />
        )}
      </FullPageHorizontalForm>
    </>
  );
};

export default CreateTemplatePage;
