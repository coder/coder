import { type Interpolation, type Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import { Link as RouterLink } from "react-router-dom";
import { type FC } from "react";
import type { TemplateExample } from "api/typesGenerated";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TemplateExampleCard } from "components/TemplateExampleCard/TemplateExampleCard";
import { docs } from "utils/docs";

// Those are from https://github.com/coder/coder/tree/main/examples/templates
const featuredExampleIds = [
  "docker",
  "kubernetes",
  "aws-linux",
  "aws-windows",
  "gcp-linux",
  "gcp-windows",
];

const findFeaturedExamples = (examples: TemplateExample[]) => {
  const featuredExamples: TemplateExample[] = [];

  // We loop the featuredExampleIds first to keep the order
  featuredExampleIds.forEach((exampleId) => {
    examples.forEach((example) => {
      if (exampleId === example.id) {
        featuredExamples.push(example);
      }
    });
  });

  return featuredExamples;
};

export const EmptyTemplates: FC<{
  canCreateTemplates: boolean;
  examples: TemplateExample[];
}> = ({ canCreateTemplates, examples }) => {
  const featuredExamples = findFeaturedExamples(examples);

  if (canCreateTemplates) {
    return (
      <TableEmpty
        message="Create your first template"
        description={
          <>
            Templates are written in Terraform and describe the infrastructure
            for workspaces. You can start using a starter template below or{" "}
            <Link
              href={docs("/templates/tutorial")}
              target="_blank"
              rel="noreferrer"
            >
              create your own
            </Link>
            .
          </>
        }
        cta={
          <Stack alignItems="center" spacing={4}>
            <div css={styles.featuredExamples}>
              {featuredExamples.map((example) => (
                <TemplateExampleCard example={example} key={example.id} />
              ))}
            </div>

            <Button
              size="small"
              component={RouterLink}
              to="/starter-templates"
              css={{ borderRadius: 9999 }}
            >
              View all starter templates
            </Button>
          </Stack>
        }
      />
    );
  }

  return (
    <TableEmpty
      css={styles.withImage}
      message="Create a Template"
      description="Contact your Coder administrator to create a template. You can share the code below."
      cta={<CodeExample code="coder templates init" />}
      image={
        <div css={styles.emptyImage}>
          <img src="/featured/templates.webp" alt="" />
        </div>
      }
    />
  );
};

const styles = {
  withImage: {
    paddingBottom: 0,
  },

  emptyImage: {
    maxWidth: "50%",
    height: 320,
    overflow: "hidden",
    opacity: 0.85,

    "& img": {
      maxWidth: "100%",
    },
  },

  featuredExamples: {
    display: "flex",
    flexWrap: "wrap",
    justifyContent: "center",
    gap: 16,
  },
} satisfies Record<string, Interpolation<Theme>>;
