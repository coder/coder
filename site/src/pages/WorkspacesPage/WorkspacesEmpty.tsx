import ArrowForwardOutlined from "@mui/icons-material/ArrowForwardOutlined";
import Button from "@mui/material/Button";
import type { FC } from "react";
import { Link } from "react-router-dom";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { linkToTemplate, useLinks } from "modules/navigation";

interface WorkspacesEmptyProps {
  isUsingFilter: boolean;
  templates?: Template[];
  canCreateTemplate: boolean;
}

export const WorkspacesEmpty: FC<WorkspacesEmptyProps> = ({
  isUsingFilter,
  templates,
  canCreateTemplate,
}) => {
  const getLink = useLinks();

  const totalFeaturedTemplates = 6;
  const featuredTemplates = templates?.slice(0, totalFeaturedTemplates);
  const defaultTitle = "Create a workspace";
  const defaultMessage =
    "A workspace is your personal, customizable development environment.";
  const defaultImage = (
    <div
      css={{
        maxWidth: "50%",
        height: 272,
        overflow: "hidden",
        marginTop: 48,
        opacity: 0.85,

        "& img": {
          maxWidth: "100%",
        },
      }}
    >
      <img src="/featured/workspaces.webp" alt="" />
    </div>
  );

  if (isUsingFilter) {
    return <TableEmpty message="No results matched your search" />;
  }

  if (templates && templates.length === 0 && canCreateTemplate) {
    return (
      <TableEmpty
        message={defaultTitle}
        description={`${defaultMessage} To create a workspace, you first need to create a template.`}
        cta={
          <Button
            component={Link}
            to="/templates"
            variant="contained"
            startIcon={<ArrowForwardOutlined />}
          >
            Go to templates
          </Button>
        }
        css={{
          paddingBottom: 0,
        }}
        image={defaultImage}
      />
    );
  }

  if (templates && templates.length === 0 && !canCreateTemplate) {
    return (
      <TableEmpty
        message={defaultTitle}
        description={`${defaultMessage} There are no templates available, but you will see them here once your admin adds them.`}
        css={{
          paddingBottom: 0,
        }}
        image={defaultImage}
      />
    );
  }

  return (
    <TableEmpty
      message={defaultTitle}
      description={`${defaultMessage} Select one template below to start.`}
      cta={
        <div>
          <div
            css={{
              display: "flex",
              flexWrap: "wrap",
              gap: 16,
              marginBottom: 24,
              justifyContent: "center",
              maxWidth: "800px",
            }}
          >
            {featuredTemplates?.map((t) => (
              <Link
                key={t.id}
                to={`${getLink(
                  linkToTemplate(t.organization_name, t.name),
                )}/workspace`}
                css={(theme) => ({
                  width: "320px",
                  padding: 16,
                  borderRadius: 6,
                  border: `1px solid ${theme.palette.divider}`,
                  textAlign: "left",
                  display: "flex",
                  gap: 16,
                  textDecoration: "none",
                  color: "inherit",

                  "&:hover": {
                    backgroundColor: theme.palette.background.paper,
                  },
                })}
              >
                <div css={{ flexShrink: 0, paddingTop: 4 }}>
                  <Avatar
                    variant={t.icon ? "square" : undefined}
                    fitImage={Boolean(t.icon)}
                    src={t.icon}
                    size="sm"
                  >
                    {t.name}
                  </Avatar>
                </div>

                <div css={{ width: "100%", minWidth: "0" }}>
                  <h4
                    css={{
                      fontSize: 14,
                      fontWeight: 600,
                      margin: 0,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {t.display_name || t.name}
                  </h4>

                  <p
                    css={(theme) => ({
                      fontSize: 13,
                      color: theme.palette.text.secondary,
                      lineHeight: "1.4",
                      margin: 0,
                      paddingTop: "4px",

                      // We've had users plug URLs directly into the
                      // descriptions, when those URLS have no hyphens or other
                      // easy semantic breakpoints. Need to set this to ensure
                      // those URLs don't break outside their containing boxes
                      wordBreak: "break-word",
                    })}
                  >
                    {t.description}
                  </p>
                </div>
              </Link>
            ))}
          </div>

          {templates && templates.length > totalFeaturedTemplates && (
            <Button
              component={Link}
              to="/templates"
              variant="contained"
              startIcon={<ArrowForwardOutlined />}
            >
              See all templates
            </Button>
          )}
        </div>
      }
    />
  );
};
