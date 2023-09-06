import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import { makeStyles } from "@mui/styles";
import { TemplateExample } from "api/typesGenerated";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TemplateExampleCard } from "components/TemplateExampleCard/TemplateExampleCard";
import { FC } from "react";
import { useTranslation } from "react-i18next";
import { Link as RouterLink } from "react-router-dom";
import { docs } from "utils/docs";
import { Permissions } from "xServices/auth/authXService";

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
  permissions: Permissions;
  examples: TemplateExample[];
}> = ({ permissions, examples }) => {
  const styles = useStyles();
  const { t } = useTranslation("templatesPage");
  const featuredExamples = findFeaturedExamples(examples);

  if (permissions.createTemplates) {
    return (
      <TableEmpty
        message={t("empty.message")}
        description={
          <>
            Templates are written in Terraform and describe the infrastructure
            for workspaces (e.g., docker_container, aws_instance,
            kubernetes_pod). Select a starter template below or
            <Link
              href={docs("/templates#add-a-template")}
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
            <div className={styles.featuredExamples}>
              {featuredExamples.map((example) => (
                <TemplateExampleCard
                  example={example}
                  key={example.id}
                  className={styles.template}
                />
              ))}
            </div>

            <Button
              size="small"
              component={RouterLink}
              to="/starter-templates"
              className={styles.viewAllButton}
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
      className={styles.withImage}
      message={t("empty.message")}
      description={t("empty.descriptionWithoutPermissions")}
      cta={<CodeExample code="coder templates init" />}
      image={
        <div className={styles.emptyImage}>
          <img src="/featured/templates.webp" alt="" />
        </div>
      }
    />
  );
};

const useStyles = makeStyles((theme) => ({
  withImage: {
    paddingBottom: 0,
  },

  emptyImage: {
    maxWidth: "50%",
    height: theme.spacing(40),
    overflow: "hidden",
    opacity: 0.85,

    "& img": {
      maxWidth: "100%",
    },
  },

  featuredExamples: {
    maxWidth: theme.spacing(100),
    display: "grid",
    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
    gap: theme.spacing(2),
    gridAutoRows: "min-content",
  },

  template: {
    backgroundColor: theme.palette.background.paperLight,

    "&:hover": {
      backgroundColor: theme.palette.divider,
    },
  },

  viewAllButton: {
    borderRadius: 9999,
  },
}));
