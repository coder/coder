import type { Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { Link, useNavigate } from "react-router-dom";
import type { Template, TemplateExample } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  HelpTooltip,
  HelpTooltipContent,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { TemplateCard } from "modules/templates/TemplateCard/TemplateCard";
import { docs } from "utils/docs";
import { CreateTemplateButton } from "../CreateTemplateButton";
import { EmptyTemplates } from "../EmptyTemplates";

export const Language = {
  templateTooltipTitle: "What is a template?",
  templateTooltipText:
    "Templates allow you to create a common configuration for your workspaces using Terraform.",
  templateTooltipLink: "Manage templates",
};

const TemplateHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTrigger />
      <HelpTooltipContent>
        <HelpTooltipTitle>{Language.templateTooltipTitle}</HelpTooltipTitle>
        <HelpTooltipText>{Language.templateTooltipText}</HelpTooltipText>
        <HelpTooltipLinksGroup>
          <HelpTooltipLink href={docs("/templates")}>
            {Language.templateTooltipLink}
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </HelpTooltipContent>
    </HelpTooltip>
  );
};

export interface TemplatesPageViewProps {
  templates: Template[] | undefined;
  examples: TemplateExample[] | undefined;
  canCreateTemplates: boolean;
  query: string | undefined;
  error?: unknown;
}

export type TemplatesByOrg = Record<
  string,
  { displayName: string; count: number }
>;

const getTemplatesByOrg = (templates: Template[]): TemplatesByOrg => {
  const orgs: TemplatesByOrg = {
    all: { displayName: "All Organizations", count: 0 },
  };

  templates.forEach((template) => {
    if (orgs[template.organization_name]) {
      orgs[template.organization_name].count += 1;
    } else {
      orgs[template.organization_name] = {
        displayName: template.organization_display_name,
        count: 1,
      };
    }

    orgs.all.count += 1;
  });

  return orgs;
};

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
  templates,
  examples,
  canCreateTemplates,
  query,
  error,
}) => {
  const navigate = useNavigate();
  const isEmpty = templates && templates.length === 0;
  const q = query?.split(":") || [];
  const activeOrg = q[0] === "organization" ? query?.split(":")[1] : "all";
  const templatesByOrg = getTemplatesByOrg(templates ?? []);

  return (
    <Margins>
      <PageHeader
        actions={
          canCreateTemplates && <CreateTemplateButton onNavigate={navigate} />
        }
      >
        <PageHeaderTitle>
          <Stack spacing={1} direction="row" alignItems="center">
            Templates
            <TemplateHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        {templates && templates.length > 0 && (
          <PageHeaderSubtitle>
            Select a template to create a workspace.
          </PageHeaderSubtitle>
        )}
      </PageHeader>

      {Boolean(error) && (
        <ErrorAlert error={error} css={{ marginBottom: 32 }} />
      )}

      <Stack direction="row" spacing={4} alignItems="flex-start">
        <Stack css={{ width: 208, flexShrink: 0, position: "sticky", top: 48 }}>
          <span css={styles.filterCaption}>ORGANIZATION</span>
          {Object.entries(templatesByOrg).map(([key, value]) => (
            <Link
              key={key}
              to={key !== "all" ? `?filter=organization:${key}` : "?"}
              css={[styles.tagLink, key === activeOrg && styles.tagLinkActive]}
            >
              {value.displayName} ({value.count ?? 0})
            </Link>
          ))}
        </Stack>

        <div
          css={{
            display: "flex",
            flexWrap: "wrap",
            gap: 32,
            height: "max-content",
          }}
        >
          {isEmpty ? (
            <EmptyTemplates
              canCreateTemplates={canCreateTemplates}
              examples={examples ?? []}
            />
          ) : (
            templates &&
            templates.map((template) => (
              <TemplateCard
                css={(theme) => ({
                  backgroundColor: theme.palette.background.paper,
                })}
                template={template}
                key={template.id}
              />
            ))
          )}
        </div>
      </Stack>
    </Margins>
  );
};

const styles = {
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
  secondary: (theme) => ({
    color: theme.palette.text.secondary,
  }),
  actionButton: (theme) => ({
    transition: "none",
    color: theme.palette.text.secondary,
    "&:hover": {
      borderColor: theme.palette.text.primary,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
