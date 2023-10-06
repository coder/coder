import { type PropsWithChildren, type ReactNode, useState } from "react";
import { useTheme } from "@emotion/react";
import { Language } from "./WorkspacesPageView";

import { type Template } from "api/typesGenerated";
import { type UseQueryResult } from "@tanstack/react-query";

import { Link as RouterLink } from "react-router-dom";
import Box from "@mui/system/Box";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import AddIcon from "@mui/icons-material/AddOutlined";
import OpenIcon from "@mui/icons-material/OpenInNewOutlined";
import Typography from "@mui/material/Typography";

import { Loader } from "components/Loader/Loader";
import { OverflowY } from "components/OverflowY/OverflowY";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Avatar } from "components/Avatar/Avatar";
import { SearchBox } from "./WorkspacesSearchBox";
import {
  PopoverContainer,
  PopoverLink,
} from "components/PopoverContainer/PopoverContainer";

const ICON_SIZE = 18;
const COLUMN_GAP = 1.5;

function sortTemplatesByUsersDesc(
  templates: readonly Template[],
  searchTerm: string,
) {
  const allWhitespace = /^\s+$/.test(searchTerm);
  if (allWhitespace) {
    return templates;
  }

  const termMatcher = new RegExp(searchTerm.replaceAll(/[^\w]/g, "."), "i");
  return templates
    .filter((template) => termMatcher.test(template.display_name))
    .sort((t1, t2) => t2.active_user_count - t1.active_user_count)
    .slice(0, 10);
}

function WorkspaceResultsRow({ template }: { template: Template }) {
  const theme = useTheme();

  return (
    <PopoverLink to={`/templates/${template.name}/workspace`}>
      <Box
        sx={{
          display: "flex",
          columnGap: COLUMN_GAP,
          alignItems: "center",
          paddingX: 2,
          paddingY: 1,
          overflowY: "hidden",
        }}
      >
        <Avatar
          src={template.icon}
          fitImage
          alt={template.display_name || "Coder template"}
          sx={{
            width: `${ICON_SIZE}px`,
            height: `${ICON_SIZE}px`,
            fontSize: `${ICON_SIZE * 0.5}px`,
            fontWeight: 700,
          }}
        >
          {template.display_name || "-"}
        </Avatar>

        <Box
          sx={{
            lineHeight: 1,
            width: "100%",
            overflow: "hidden",
            color: "white",
          }}
        >
          <Typography
            component="p"
            sx={{ marginY: 0, paddingBottom: 0.5, lineHeight: 1 }}
            noWrap
          >
            {template.display_name || "[Unnamed]"}
          </Typography>

          <Box
            component="p"
            sx={{
              marginY: 0,
              fontSize: 14,
              color: theme.palette.text.secondary,
            }}
          >
            {/*
             * There are some templates that have -1 as their user count –
             * basically functioning like a null value in JS. Can safely just
             * treat them as if they were 0.
             */}
            {template.active_user_count <= 0
              ? "No"
              : template.active_user_count}{" "}
            developer
            {template.active_user_count === 1 ? "" : "s"}
          </Box>
        </Box>
      </Box>
    </PopoverLink>
  );
}

type TemplatesQuery = UseQueryResult<Template[]>;

type WorkspacesButtonProps = PropsWithChildren<{
  templatesFetchStatus: TemplatesQuery["status"];
  templates: TemplatesQuery["data"];
}>;

export function WorkspacesButton({
  children,
  templatesFetchStatus,
  templates,
}: WorkspacesButtonProps) {
  const theme = useTheme();

  // Dataset should always be small enough that client-side filtering should be
  // good enough. Can swap out down the line if it becomes an issue
  const [searchTerm, setSearchTerm] = useState("");
  const processed = sortTemplatesByUsersDesc(templates ?? [], searchTerm);

  let emptyState: ReactNode = undefined;
  if (templates?.length === 0) {
    emptyState = (
      <EmptyState
        message="No templates yet"
        cta={
          <Link to="/templates" component={RouterLink}>
            Create one now.
          </Link>
        }
      />
    );
  } else if (processed.length === 0) {
    emptyState = <EmptyState message="No templates match your text" />;
  }

  return (
    <PopoverContainer
      // Stopgap value until bug where string-based horizontal origin isn't
      // being applied consistently can get figured out
      originX={-115}
      originY="bottom"
      sx={{ display: "flex", flexFlow: "column nowrap" }}
      anchorButton={
        <Button startIcon={<AddIcon />} variant="contained">
          {children}
        </Button>
      }
    >
      <SearchBox
        value={searchTerm}
        onValueChange={(newValue) => setSearchTerm(newValue)}
        placeholder="Type/select a workspace template"
        label="Template select for workspace"
        sx={{ flexShrink: 0, columnGap: COLUMN_GAP }}
      />

      <OverflowY
        maxHeight={380}
        sx={{
          display: "flex",
          flexFlow: "column nowrap",
          paddingY: 1,
        }}
      >
        {templatesFetchStatus === "loading" ? (
          <Loader size={14} />
        ) : (
          <>
            {processed.map((template) => (
              <WorkspaceResultsRow key={template.id} template={template} />
            ))}

            {emptyState}
          </>
        )}
      </OverflowY>

      <Link
        component={RouterLink}
        to="/templates"
        sx={{
          outline: "none",
          "&:focus": {
            backgroundColor: theme.palette.action.focus,
          },
        }}
      >
        <Box
          sx={{
            padding: 2,
            display: "flex",
            flexFlow: "row nowrap",
            alignItems: "center",
            columnGap: COLUMN_GAP,
            borderTop: `1px solid ${theme.palette.divider}`,
          }}
        >
          <Box component="span" sx={{ width: `${ICON_SIZE}px` }}>
            <OpenIcon
              sx={{ fontSize: "16px", marginX: "auto", display: "block" }}
            />
          </Box>
          <span>{Language.seeAllTemplates}</span>
        </Box>
      </Link>
    </PopoverContainer>
  );
}
