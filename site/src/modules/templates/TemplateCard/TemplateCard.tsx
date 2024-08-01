import type { Interpolation, Theme } from "@emotion/react";
import ArrowForwardOutlined from "@mui/icons-material/ArrowForwardOutlined";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import { visuallyHidden } from "@mui/utils";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import type { Template } from "api/typesGenerated";
import { ExternalAvatar, Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { DeprecatedBadge } from "components/Badges/Badges";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Pill } from "components/Pill/Pill";
import { formatTemplateBuildTime } from "utils/templates";

type TemplateCardProps = HTMLAttributes<HTMLDivElement> & {
  template: Template;
  activeOrg?: string;
  hasMultipleOrgs: boolean;
};

export const TemplateCard: FC<TemplateCardProps> = ({
  template,
  activeOrg,
  hasMultipleOrgs,
  ...divProps
}) => {
  const navigate = useNavigate();
  const templatePageLink = `/templates/${template.name}`;
  const hasIcon = template.icon && template.icon !== "";

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && e.currentTarget === e.target) {
      navigate(templatePageLink);
    }
  };
  return (
    <div
      css={styles.card}
      {...divProps}
      role="button"
      tabIndex={0}
      onClick={() => navigate(templatePageLink)}
      onKeyDown={handleKeyDown}
    >
      <div css={styles.header}>
        <div css={{ display: "flex", alignItems: "center" }}>
          <AvatarData
            displayTitle={false}
            subtitle=""
            title={
              template.display_name.length > 0
                ? template.display_name
                : template.name
            }
            avatar={hasIcon && <Avatar src={template.icon} size="xl" />}
          />
          <p
            css={(theme) => ({
              fontSize: 13,
              margin: "0 0 0 auto",
              color: theme.palette.text.secondary,
            })}
          >
            <span css={{ ...visuallyHidden }}>Build time: </span>
            <Tooltip title="Build time" placement="bottom-start">
              <span>
                {formatTemplateBuildTime(template.build_time_stats.start.P50)}
              </span>
            </Tooltip>
          </p>
        </div>

        {hasMultipleOrgs && (
          <div css={styles.orgs}>
            <RouterLink
              to={`/organizations/${template.organization_name}`}
              onClick={(e) => e.stopPropagation()}
            >
              <Pill
                css={[
                  styles.org,
                  activeOrg === template.organization_id && styles.activeOrg,
                ]}
              >
                {template.organization_display_name}
              </Pill>
            </RouterLink>
          </div>
        )}
      </div>

      <div>
        <h4 css={{ fontSize: 14, fontWeight: 600, margin: 0, marginBottom: 4 }}>
          {template.display_name}
        </h4>
        <span css={styles.description}>
          {template.description}{" "}
          <Link
            component={RouterLink}
            onClick={(e) => e.stopPropagation()}
            to={`/templates/${template.name}/docs`}
            css={{ display: "inline-block", fontSize: 13, marginTop: 4 }}
          >
            Read more
          </Link>
        </span>
      </div>

      <div css={styles.useButtonContainer}>
        {template.deprecated ? (
          <DeprecatedBadge />
        ) : (
          <Button
            component={RouterLink}
            onClick={(e) => e.stopPropagation()}
            fullWidth
            to={`/templates/${template.name}/workspace`}
          >
            Use template
          </Button>
        )}
      </div>
    </div>
  );
};

const styles = {
  card: (theme) => ({
    width: "320px",
    padding: 24,
    borderRadius: 6,
    border: `1px solid ${theme.palette.divider}`,
    textAlign: "left",
    color: "inherit",
    display: "flex",
    flexDirection: "column",
    cursor: "pointer",
    "&:hover": {
      color: theme.experimental.l2.hover.text,
      borderColor: theme.experimental.l2.hover.text,
    },
  }),

  header: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 24,
  },

  icon: {
    flexShrink: 0,
    paddingTop: 4,
    width: 32,
    height: 32,
  },

  description: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    lineHeight: "1.6",
    display: "block",
  }),

  useButtonContainer: {
    display: "flex",
    gap: 12,
    flexDirection: "column",
    paddingTop: 24,
    marginTop: "auto",
    alignItems: "center",
  },

  actionButton: (theme) => ({
    transition: "none",
    color: theme.palette.text.secondary,
    "&:hover": {
      borderColor: theme.palette.text.primary,
    },
  }),

  orgs: {
    display: "flex",
    flexWrap: "wrap",
    gap: 8,
    justifyContent: "end",
  },

  org: (theme) => ({
    borderColor: theme.palette.divider,
    textDecoration: "none",
    cursor: "pointer",
    "&: hover": {
      borderColor: theme.palette.primary.main,
    },
  }),

  activeOrg: (theme) => ({
    borderColor: theme.roles.active.outline,
    backgroundColor: theme.roles.active.background,
  }),
} satisfies Record<string, Interpolation<Theme>>;
