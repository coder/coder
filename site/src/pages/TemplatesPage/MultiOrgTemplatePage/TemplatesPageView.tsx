import type { Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import type { TemplateExample } from "api/typesGenerated";
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
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { TemplateCard } from "modules/templates/TemplateCard/TemplateCard";
import { docs } from "utils/docs";
import type { TemplatesByOrg } from "utils/templateAggregators";
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
  templatesByOrg?: TemplatesByOrg;
  examples: TemplateExample[] | undefined;
  canCreateTemplates: boolean;
  error?: unknown;
}

const sortOrgs = (templatesByOrg: TemplatesByOrg) => {
  return templatesByOrg
    ? Object.keys(templatesByOrg).sort((a, b) => a.localeCompare(b))
    : undefined;
};

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
  templatesByOrg,
  examples,
  canCreateTemplates,
  error,
}) => {
  const navigate = useNavigate();
  const [urlParams] = useSearchParams();
  const isEmpty = templatesByOrg && templatesByOrg["all"].length === 0;
  const orgs = templatesByOrg ? sortOrgs(templatesByOrg) : undefined;
  const activeOrg = urlParams.get("org") ?? "all";
  const visibleTemplates = templatesByOrg
    ? templatesByOrg[activeOrg]
    : undefined;

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
        {!isEmpty && (
          <PageHeaderSubtitle>
            Select a template to create a workspace.
          </PageHeaderSubtitle>
        )}
      </PageHeader>

      {Boolean(error) && (
        <ErrorAlert error={error} css={{ marginBottom: 32 }} />
      )}

      {Boolean(!templatesByOrg) && <Loader />}

      <Stack direction="row" spacing={4} alignItems="flex-start">
        {templatesByOrg && orgs && orgs.length > 2 && (
          <Stack
            css={{ width: 208, flexShrink: 0, position: "sticky", top: 48 }}
          >
            <span css={styles.filterCaption}>ORGANIZATION</span>
            {orgs.map((org) => (
              <Link
                key={org}
                to={`?org=${org}`}
                css={[
                  styles.tagLink,
                  org === activeOrg && styles.tagLinkActive,
                ]}
              >
                {org === "all"
                  ? org
                  : templatesByOrg[org][0].organization_display_name}
                ({templatesByOrg[org].length})
              </Link>
            ))}
          </Stack>
        )}

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
            visibleTemplates &&
            visibleTemplates.map((template) => (
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
