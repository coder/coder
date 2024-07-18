import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import CardActionArea from "@mui/material/CardActionArea";
import CardContent from "@mui/material/CardContent";
import Stack from "@mui/material/Stack";
import { useState, type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
// import { useDashboard } from "modules/dashboard/useDashboard";
import type { StarterTemplatesByTag } from "utils/starterTemplates";
import { StarterTemplates } from "./StarterTemplates";

export interface CreateTemplatePageViewProps {
  starterTemplatesByTag?: StarterTemplatesByTag;
  error?: unknown;
}

export const CreateTemplatesPageView: FC<CreateTemplatePageViewProps> = ({
  starterTemplatesByTag,
  error,
}) => {
  const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);
  // const { organizationId } = useDashboard();
  // TODO: if there is only 1 organization, set the dropdown to the default organizationId

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>Create a Template</PageHeaderTitle>
      </PageHeader>
      <Stack spacing={8}>
        <Stack direction="row" spacing={7}>
          <h2 css={styles.sectionTitle}>Choose an Organization</h2>
          <OrganizationAutocomplete
            css={styles.autoComplete}
            value={selectedOrg}
            onChange={(newValue) => {
              setSelectedOrg(newValue);
            }}
          />
        </Stack>

        <Stack direction="row" spacing={7}>
          <h2 css={styles.sectionTitle}>Choose a starting point</h2>
          <div
            css={{
              display: "flex",
              flexWrap: "wrap",
              gap: 32,
              height: "max-content",
            }}
          >
            <Card variant="outlined" sx={{ width: 320 }}>
              <CardActionArea
                component={RouterLink}
                to="../templates/new?exampleId=scratch"
                sx={{ height: 115, padding: 1 }}
              >
                <CardContent>
                  <Stack
                    direction="row"
                    spacing={3}
                    css={{ alignItems: "center" }}
                  >
                    <div css={styles.icon}>
                      <ExternalImage
                        src="/emojis/1f4c4.png"
                        css={{
                          width: "100%",
                          height: "100%",
                        }}
                      />
                    </div>
                    <div>
                      <h4 css={styles.cardTitle}>Scratch Template</h4>
                      <span css={styles.cardDescription}>
                        Create a minimal starter template that you can customize
                      </span>
                    </div>
                  </Stack>
                </CardContent>
              </CardActionArea>
            </Card>
            <Card variant="outlined" sx={{ width: 320 }}>
              <CardActionArea
                component={RouterLink}
                to="../templates/new"
                sx={{ height: 115, padding: 1 }}
              >
                <CardContent>
                  <Stack
                    direction="row"
                    spacing={3}
                    css={{ alignItems: "center" }}
                  >
                    <div css={styles.icon}>
                      <ExternalImage
                        src="/emojis/1f4e1.png"
                        css={{
                          width: "100%",
                          height: "100%",
                        }}
                      />
                    </div>
                    <div>
                      <h4 css={styles.cardTitle}>Upload Template</h4>
                      <span css={styles.cardDescription}>
                        Get started by uploading an existing template
                      </span>
                    </div>
                  </Stack>
                </CardContent>
              </CardActionArea>
            </Card>
          </div>
        </Stack>

        {Boolean(error) && <ErrorAlert error={error} />}

        {Boolean(!starterTemplatesByTag) && <Loader />}

        <StarterTemplates starterTemplatesByTag={starterTemplatesByTag} />
      </Stack>
    </Margins>
  );
};

const styles = {
  autoComplete: {
    width: 415,
  },

  sectionTitle: (theme) => ({
    color: theme.palette.text.primary,
    fontSize: 16,
    fontWeight: 400,
    margin: 0,
  }),

  cardTitle: (theme) => ({
    fontSize: 14,
    fontWeight: 600,
    margin: 0,
    marginBottom: 4,
  }),

  cardDescription: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    lineHeight: "1.6",
    display: "block",
  }),

  icon: {
    flexShrink: 0,
    width: 32,
    height: 32,
  },

  menuItemIcon: (theme) => ({
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  }),
} satisfies Record<string, Interpolation<Theme>>;
