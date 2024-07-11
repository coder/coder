import type { Interpolation, Theme } from "@emotion/react";
import ArrowForwardOutlined from "@mui/icons-material/ArrowForwardOutlined";
import Button from "@mui/material/Button";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import type { Template } from "api/typesGenerated";
import { ExternalAvatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { DeprecatedBadge } from "components/Badges/Badges";

type TemplateCardProps = HTMLAttributes<HTMLDivElement> & {
  template: Template;
};

export const TemplateCard: FC<TemplateCardProps> = ({
  template,
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
        <div>
          <AvatarData
            title={
              template.display_name.length > 0
                ? template.display_name
                : template.name
            }
            subtitle={template.organization_display_name}
            avatar={
              hasIcon && (
                <ExternalAvatar variant="square" fitImage src={template.icon} />
              )
            }
          />
        </div>
        <div>
          {template.active_user_count}{" "}
          {template.active_user_count === 1 ? "user" : "users"}
        </div>
      </div>

      <div>
        <span css={styles.description}>
          <p>{template.description}</p>
        </span>
      </div>

      <div css={styles.useButtonContainer}>
        {template.deprecated ? (
          <DeprecatedBadge />
        ) : (
          <Button
            component={RouterLink}
            css={styles.actionButton}
            className="actionButton"
            fullWidth
            startIcon={<ArrowForwardOutlined />}
            title={`Create a workspace using the ${template.display_name} template`}
            to={`/templates/${template.name}/workspace`}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            Create Workspace
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
} satisfies Record<string, Interpolation<Theme>>;
