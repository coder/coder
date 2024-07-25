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

  return (
    <div
      css={styles.card}
      {...divProps}
      role="button"
      tabIndex={0}
      onClick={() => navigate(templatePageLink)}
      onKeyDown={(event) => {
        if (event.key === "Enter" && event.currentTarget === event.target) {
          navigate(templatePageLink);
        }
      }}
    >
      <AvatarData
        css={{ lineHeight: "1.3" }}
        title={template.display_name || template.name}
        subtitle={
          <>
            {template.active_user_count}{" "}
            {template.active_user_count === 1 ? "user" : "users"} &middot;{" "}
            {template.organization_display_name}
          </>
        }
        avatar={
          hasIcon && (
            <ExternalAvatar
              variant="square"
              fitImage
              src={template.icon}
              css={{ width: "32px" }}
            />
          )
        }
      />

      <p css={styles.description}>{template.description}</p>

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
            // Stopping propagation immediately because Button will handle route
            // transitions internally
            onClick={(e) => e.stopPropagation()}
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
    flexBasis: "360px",
    flexGrow: 1,
    padding: 24,
    borderRadius: 6,
    border: `1px solid ${theme.palette.divider}`,
    textAlign: "left",
    color: "inherit",
    cursor: "pointer",
    wordBreak: "break-word",

    "&:hover": {
      borderColor: theme.experimental.l2.hover.outline,
    },
  }),

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
    alignItems: "center",

    marginTop: "auto",
    marginLeft: "auto",
    marginRight: "auto",
  },

  actionButton: (theme) => ({
    paddingLeft: "16px",
    paddingRight: "16px",
    width: "100%",
    maxWidth: "256px",
    transition: "none",
    color: theme.palette.text.secondary,

    "&:hover": {
      borderColor: theme.experimental.l2.hover.outline,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
