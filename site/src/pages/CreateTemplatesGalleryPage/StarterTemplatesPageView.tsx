import type { FC } from "react";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import type { StarterTemplatesByTag } from "utils/templateAggregators";
import { StarterTemplates } from "./StarterTemplates";

export interface StarterTemplatesPageViewProps {
  starterTemplatesByTag?: StarterTemplatesByTag;
  error?: unknown;
}

export const StarterTemplatesPageView: FC<StarterTemplatesPageViewProps> = ({
  starterTemplatesByTag,
  error,
}) => {
  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>Starter Templates</PageHeaderTitle>
        <PageHeaderSubtitle>
          Import a built-in template to start developing in the cloud
        </PageHeaderSubtitle>
      </PageHeader>

      {Boolean(error) && <ErrorAlert error={error} />}

      {Boolean(!starterTemplatesByTag) && <Loader />}

      <StarterTemplates starterTemplatesByTag={starterTemplatesByTag} />
    </Margins>
  );
};
