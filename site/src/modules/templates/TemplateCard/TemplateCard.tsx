import type { Interpolation, Theme } from "@emotion/react";
import AddCircleOutlineIcon from "@mui/icons-material/AddCircleOutline";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import { visuallyHidden } from "@mui/utils";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { DeprecatedBadge } from "components/Badges/Badges";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { createDayString } from "utils/createDayString";
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

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && e.currentTarget === e.target) {
      navigate(templatePageLink);
    }
  };

  const truncatedDescription =
    template.description.length >= 60
      ? template.description.substring(0, 60) + "..."
      : template.description;

  return (
    <div
      css={styles.card}
      role="button"
      onClick={() => navigate(templatePageLink)}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      {...divProps}
    >
      <Stack
        alignItems="center"
        justifyContent="space-between"
        direction="row"
        css={{ marginBottom: 24 }}
      >
        <AvatarData
          displayTitle={false}
          subtitle=""
          title={
            template.display_name.length > 0
              ? template.display_name
              : template.name
          }
          avatar={
            template.icon &&
            template.icon !== "" && <Avatar src={template.icon} size="md" />
          }
        />

        {hasMultipleOrgs && (
          <div css={styles.orgs}>
            <RouterLink
              to={`/templates?org=${template.organization_id}`}
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
      </Stack>

      <Stack justifyContent="space-between" css={{ height: "100%" }}>
        <Stack direction="column" spacing={0}>
          <h4
            css={{ fontSize: 14, fontWeight: 600, margin: 0, marginBottom: 4 }}
          >
            {template.display_name}
          </h4>

          {template.description && (
            <div css={styles.description}>
              {truncatedDescription}{" "}
              <Link
                component={RouterLink}
                onClick={(e) => e.stopPropagation()}
                to={`${templatePageLink}/docs`}
                css={{ display: "inline-block", fontSize: 13, marginTop: 4 }}
              >
                Read more
              </Link>
            </div>
          )}
        </Stack>

        <Stack direction="column" alignItems="flex-start" spacing={1}>
          <Stack direction="row">
            <span css={{ ...visuallyHidden }}>Used by</span>
            <Tooltip title="Used by" placement="bottom-start">
              <span css={styles.templateStat}>
                {`${template.active_user_count} ${
                  template.active_user_count === 1 ? "user" : "users"
                }`}
              </span>
            </Tooltip>
            <Divider orientation="vertical" variant="middle" flexItem />
            <span css={{ ...visuallyHidden }}>Build time</span>
            <Tooltip title="Build time" placement="bottom-start">
              <span css={styles.templateStat}>
                {`${formatTemplateBuildTime(
                  template.build_time_stats.start.P50,
                )}`}
              </span>
            </Tooltip>
            <Divider orientation="vertical" variant="middle" flexItem />
            <span css={{ ...visuallyHidden }}>Last updated</span>
            <Tooltip title="Last updated" placement="bottom-start">
              <span css={styles.templateStat}>
                {`${createDayString(template.updated_at)}`}
              </span>
            </Tooltip>
          </Stack>
          {template.deprecated ? (
            <DeprecatedBadge />
          ) : (
            <Button
              component={RouterLink}
              onClick={(e) => e.stopPropagation()}
              fullWidth
              size="small"
              startIcon={<AddCircleOutlineIcon />}
              title={`Create a workspace using the ${template.display_name} template`}
              to={`${templatePageLink}/workspace`}
            >
              Create workspace
            </Button>
          )}
        </Stack>
      </Stack>
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

  templateStat: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
  }),

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
