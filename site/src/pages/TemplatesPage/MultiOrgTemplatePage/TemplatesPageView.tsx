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

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
  templatesByOrg,
  examples,
  canCreateTemplates,
  error,
}) => {
  const sidebarWidth = 208;
  const navigate = useNavigate();
  const [urlParams] = useSearchParams();
  const isEmpty = templatesByOrg && templatesByOrg["all"].length === 0;
  const activeOrg = urlParams.get("org") ?? "all";
  const visibleTemplates = templatesByOrg
    ? templatesByOrg[activeOrg]
    : undefined;
  const hasMultipleOrgs = Boolean(
    templatesByOrg && Object.keys(templatesByOrg).length > 2,
  );

  const headerActionElem = canCreateTemplates && (
    <div
      css={(theme) => ({
        display: "flex",

        // we have special breakpoint behavior for the primary template CTA
        // so it always aligns with the cards in the gallery view
        // (as long as we have cards)
        ...(isEmpty
          ? {}
          : {
              [theme.breakpoints.down(1280)]: {
                marginLeft: "unset",
                position: "relative",
                right: 95,
              },
            }),

        [theme.breakpoints.down(1024)]: {
          position: "static",
          marginLeft: "initial",
          width: "100%",
        },
      })}
    >
      <CreateTemplateButton onNavigate={navigate} />
    </div>
  );

  return (
    <Margins>
      <div
        css={(theme) => ({
          display: "flex",
          flexDirection: "column",
          width: "100%",
          padding: hasMultipleOrgs
            ? undefined
            : `0 calc(${(sidebarWidth + 100) / 2}px)`,

          [theme.breakpoints.down(1440)]: {
            padding: undefined,
          },
        })}
      >
        <PageHeader
          css={{ display: "flex", width: "100%" }}
          actions={headerActionElem}
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

        <div
          css={{
            display: "flex",
            flexDirection: "row",
            justifyContent:
              visibleTemplates && visibleTemplates.length > 2
                ? "space-between"
                : "flex-start",
            width: "100%",
          }}
        >
          {hasMultipleOrgs && (
            <div css={{ display: "flex" }}>
              <Stack
                css={(theme) => ({
                  width: sidebarWidth,
                  flexShrink: 0,
                  position: "sticky",
                  top: 48,

                  // increase sidebar width on large screens
                  // so gap between template cards isn't too large
                  [theme.breakpoints.up(1440)]: {
                    width: `calc(${sidebarWidth}px + 100px)`,
                  },
                })}
              >
                <span css={styles.filterCaption}>ORGANIZATION</span>
                {templatesByOrg &&
                  Object.entries(templatesByOrg).map((org) => (
                    <Link
                      key={org[0]}
                      to={`?org=${org[0]}`}
                      css={[
                        styles.tagLink,
                        org[0] === activeOrg && styles.tagLinkActive,
                      ]}
                    >
                      {org[0] === "all"
                        ? "all"
                        : org[1][0].organization_display_name}{" "}
                      ({org[1].length})
                    </Link>
                  ))}
              </Stack>
            </div>
          )}

          <div css={{ display: "flex" }}>
            <div
              css={(theme) => ({
                display: "flex",
                justifyContent: "flex-start",
                flexWrap: "wrap",
                gap: 32,
                height: "max-content",

                [theme.breakpoints.down(1280)]: {
                  justifyContent: "flex-start",
                },
              })}
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
                      height: 300,
                      backgroundColor: theme.palette.background.paper,
                    })}
                    template={template}
                    activeOrg={activeOrg}
                    hasMultipleOrgs={hasMultipleOrgs}
                    key={template.id}
                  />
                ))
              )}
            </div>
          </div>
        </div>
      </div>
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
