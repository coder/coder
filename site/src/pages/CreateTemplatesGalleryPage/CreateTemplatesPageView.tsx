import type { Interpolation, Theme } from "@emotion/react";
import { useState, type FC } from "react";
import { useQuery } from "react-query";
import { Link, useSearchParams } from "react-router-dom";
import { templateExamples } from "api/queries/templates";
import type { Organization, TemplateExample } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { useDashboard } from "modules/dashboard/useDashboard";
import { TemplateExampleCard } from "modules/templates/TemplateExampleCard/TemplateExampleCard";
import {
  getTemplatesByTag,
  type StarterTemplatesByTag,
} from "utils/starterTemplates";
import { StarterTemplates } from "./StarterTemplates";

// const getTagLabel = (tag: string) => {
//   const labelByTag: Record<string, string> = {
//     all: "All templates",
//     digitalocean: "DigitalOcean",
//     aws: "AWS",
//     google: "Google Cloud",
//   };
//   // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
//   return labelByTag[tag] ?? tag;
// };

// const selectTags = (starterTemplatesByTag: StarterTemplatesByTag) => {
//   return starterTemplatesByTag
//     ? Object.keys(starterTemplatesByTag).sort((a, b) => a.localeCompare(b))
//     : undefined;
// };

export interface CreateTemplatePageViewProps {
  starterTemplatesByTag?: StarterTemplatesByTag;
  error?: unknown;
}

// const removeScratchExample = (data: TemplateExample[]) => {
//   return data.filter((example) => example.id !== "scratch");
// };

export const CreateTemplatesPageView: FC<CreateTemplatePageViewProps> = ({
  starterTemplatesByTag,
  error,
}) => {
  const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);
  // const { organizationId } = useDashboard();
  // const templateExamplesQuery = useQuery(templateExamples(organizationId));
  // const starterTemplatesByTag = templateExamplesQuery.data
  //   ? // Currently, the scratch template should not be displayed on the starter templates page.
  //     getTemplatesByTag(removeScratchExample(templateExamplesQuery.data))
  //   : undefined;

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>Create a Template</PageHeaderTitle>
      </PageHeader>

      <OrganizationAutocomplete
        css={styles.autoComplete}
        value={selectedOrg}
        onChange={(newValue) => {
          setSelectedOrg(newValue);
        }}
      />

      {Boolean(error) && <ErrorAlert error={error} />}

      {Boolean(!starterTemplatesByTag) && <Loader />}

      <StarterTemplates starterTemplatesByTag={starterTemplatesByTag} />
    </Margins>
  );
};

const styles = {
  autoComplete: {
    width: 300,
  },

  filterCaption: (theme) => ({
    textTransform: "uppercase",
    fontWeight: 600,
    fontSize: 12,
    color: theme.palette.text.secondary,
    letterSpacing: "0.1em",
  }),

  tagLink: (theme) => ({
    color: theme.palette.text.secondary,
    textDecoration: "none",
    fontSize: 14,
    textTransform: "capitalize",

    "&:hover": {
      color: theme.palette.text.primary,
    },
  }),

  tagLinkActive: (theme) => ({
    color: theme.palette.text.primary,
    fontWeight: 600,
  }),
} satisfies Record<string, Interpolation<Theme>>;
