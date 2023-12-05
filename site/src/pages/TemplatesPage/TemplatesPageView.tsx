import { type Interpolation, type Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import AddIcon from "@mui/icons-material/AddOutlined";
import { type FC } from "react";
import { useNavigate, Link as RouterLink } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import {
  formatTemplateBuildTime,
  formatTemplateActiveDevelopers,
} from "utils/templates";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import {
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { EmptyTemplates } from "./EmptyTemplates";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import type { Template, TemplateExample } from "api/typesGenerated";
import { colors } from "theme/colors";
import ArrowForwardOutlined from "@mui/icons-material/ArrowForwardOutlined";
import { Avatar } from "components/Avatar/Avatar";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { docs } from "utils/docs";
import Skeleton from "@mui/material/Skeleton";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";
import { Pill } from "components/Pill/Pill";

export const Language = {
  developerCount: (activeCount: number): string => {
    return `${formatTemplateActiveDevelopers(activeCount)} developer${
      activeCount !== 1 ? "s" : ""
    }`;
  },
  nameLabel: "Name",
  buildTimeLabel: "Build time",
  usedByLabel: "Used by",
  lastUpdatedLabel: "Last updated",
  templateTooltipTitle: "What is template?",
  templateTooltipText:
    "With templates you can create a common configuration for your workspaces using Terraform.",
  templateTooltipLink: "Manage templates",
};

const TemplateHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.templateTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.templateTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href={docs("/templates")}>
          {Language.templateTooltipLink}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  );
};

interface TemplateRowProps {
  template: Template;
}

const TemplateRow: FC<TemplateRowProps> = ({ template }) => {
  const templatePageLink = `/templates/${template.name}`;
  const hasIcon = template.icon && template.icon !== "";
  const navigate = useNavigate();

  const { css: clickableCss, ...clickableRow } = useClickableTableRow({
    onClick: () => navigate(templatePageLink),
  });

  return (
    <TableRow
      key={template.id}
      data-testid={`template-${template.id}`}
      {...clickableRow}
      css={[clickableCss, styles.tableRow]}
    >
      <TableCell>
        <AvatarData
          title={
            template.display_name.length > 0
              ? template.display_name
              : template.name
          }
          subtitle={template.description}
          avatar={
            hasIcon && <Avatar src={template.icon} variant="square" fitImage />
          }
        />
      </TableCell>

      <TableCell css={styles.secondary}>
        {Language.developerCount(template.active_user_count)}
      </TableCell>

      <TableCell css={styles.secondary}>
        {formatTemplateBuildTime(template.build_time_stats.start.P50)}
      </TableCell>

      <TableCell data-chromatic="ignore" css={styles.secondary}>
        {createDayString(template.updated_at)}
      </TableCell>

      <TableCell css={styles.actionCell}>
        {template.deprecated ? (
          <Pill text="Deprecated" type="warning" />
        ) : (
          <Button
            size="small"
            css={styles.actionButton}
            className="actionButton"
            startIcon={<ArrowForwardOutlined />}
            title={`Create a workspace using the ${template.display_name} template`}
            onClick={(e) => {
              e.stopPropagation();
              navigate(`/templates/${template.name}/workspace`);
            }}
          >
            Create Workspace
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
};

export interface TemplatesPageViewProps {
  error?: unknown;
  examples: TemplateExample[] | undefined;
  templates: Template[] | undefined;
  canCreateTemplates: boolean;
}

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
  templates,
  error,
  examples,
  canCreateTemplates,
}) => {
  const isLoading = !templates;
  const isEmpty = templates && templates.length === 0;

  return (
    <Margins>
      <PageHeader
        actions={
          canCreateTemplates && (
            <>
              <Button component={RouterLink} to="/starter-templates">
                Starter Templates
              </Button>
              <Button
                startIcon={<AddIcon />}
                component={RouterLink}
                to="new"
                variant="contained"
              >
                Create Template
              </Button>
            </>
          )
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

      {error ? (
        <ErrorAlert error={error} />
      ) : (
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell width="35%">{Language.nameLabel}</TableCell>
                <TableCell width="15%">{Language.usedByLabel}</TableCell>
                <TableCell width="10%">{Language.buildTimeLabel}</TableCell>
                <TableCell width="15%">{Language.lastUpdatedLabel}</TableCell>
                <TableCell width="1%"></TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading && <TableLoader />}

              {isEmpty ? (
                <EmptyTemplates
                  canCreateTemplates={canCreateTemplates}
                  examples={examples ?? []}
                />
              ) : (
                templates?.map((template) => (
                  <TemplateRow key={template.id} template={template} />
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Margins>
  );
};

const TableLoader: FC = () => {
  return (
    <TableLoaderSkeleton>
      <TableRowSkeleton>
        <TableCell>
          <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
            <AvatarDataSkeleton />
          </div>
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
      </TableRowSkeleton>
    </TableLoaderSkeleton>
  );
};

const styles = {
  templateIconWrapper: {
    // Same size then the avatar component
    width: 36,
    height: 36,
    padding: 2,

    "& img": {
      width: "100%",
    },
  },
  actionCell: {
    whiteSpace: "nowrap",
  },
  secondary: (theme) => ({
    color: theme.palette.text.secondary,
  }),
  tableRow: (theme) => ({
    "&:hover .actionButton": {
      color: theme.palette.text.primary,
      borderColor: colors.gray[11],
      "&:hover": {
        borderColor: theme.palette.text.primary,
      },
    },
  }),
  actionButton: (theme) => ({
    color: theme.palette.text.secondary,
    transition: "none",
  }),
} satisfies Record<string, Interpolation<Theme>>;
