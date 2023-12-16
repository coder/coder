import { type FC, type ReactNode, useState } from "react";
import { type Template } from "api/typesGenerated";
import { type UseQueryResult } from "react-query";
import {
  Link as RouterLink,
  LinkProps as RouterLinkProps,
} from "react-router-dom";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import AddIcon from "@mui/icons-material/AddOutlined";
import OpenIcon from "@mui/icons-material/OpenInNewOutlined";
import { Loader } from "components/Loader/Loader";
import { OverflowY } from "components/OverflowY/OverflowY";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Avatar } from "components/Avatar/Avatar";
import { SearchBox } from "./WorkspacesSearchBox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

const ICON_SIZE = 18;

type TemplatesQuery = UseQueryResult<Template[]>;

interface WorkspacesButtonProps {
  children?: ReactNode;
  templatesFetchStatus: TemplatesQuery["status"];
  templates: TemplatesQuery["data"];
}

export const WorkspacesButton: FC<WorkspacesButtonProps> = ({
  children,
  templatesFetchStatus,
  templates,
}) => {
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
    <Popover>
      <PopoverTrigger>
        <Button startIcon={<AddIcon />} variant="contained">
          {children}
        </Button>
      </PopoverTrigger>
      <PopoverContent horizontal="right">
        <SearchBox
          value={searchTerm}
          onValueChange={(newValue) => setSearchTerm(newValue)}
          placeholder="Type/select a workspace template"
          label="Template select for workspace"
          css={{ flexShrink: 0, columnGap: 12 }}
        />

        <OverflowY
          maxHeight={380}
          css={{
            display: "flex",
            flexDirection: "column",
            paddingTop: "8px",
            paddingBottom: "8px",
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

        <div
          css={(theme) => ({
            padding: "8px 0",
            borderTop: `1px solid ${theme.palette.divider}`,
          })}
        >
          <PopoverLink
            to="/templates"
            css={(theme) => ({
              display: "flex",
              alignItems: "center",
              columnGap: 12,
              color: theme.palette.primary.main,
            })}
          >
            <OpenIcon css={{ width: 14, height: 14 }} />
            <span>See all templates</span>
          </PopoverLink>
        </div>
      </PopoverContent>
    </Popover>
  );
};

interface WorkspaceResultsRowProps {
  template: Template;
}

const WorkspaceResultsRow: FC<WorkspaceResultsRowProps> = ({ template }) => {
  return (
    <PopoverLink
      to={`/templates/${template.name}/workspace`}
      css={{
        display: "flex",
        gap: 12,
        alignItems: "center",
      }}
    >
      <Avatar
        src={template.icon}
        fitImage
        alt={template.display_name || "Coder template"}
        css={{
          width: `${ICON_SIZE}px`,
          height: `${ICON_SIZE}px`,
          fontSize: `${ICON_SIZE * 0.5}px`,
          fontWeight: 700,
        }}
      >
        {template.display_name || "-"}
      </Avatar>

      <div
        css={(theme) => ({
          color: theme.palette.text.primary,
          display: "flex",
          flexDirection: "column",
          lineHeight: "140%",
          fontSize: 14,
          overflow: "hidden",
        })}
      >
        <span css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}>
          {template.display_name || template.name || "[Unnamed]"}
        </span>
        <span
          css={(theme) => ({
            fontSize: 13,
            color: theme.palette.text.secondary,
          })}
        >
          {/*
           * There are some templates that have -1 as their user count –
           * basically functioning like a null value in JS. Can safely just
           * treat them as if they were 0.
           */}
          {template.active_user_count <= 0 ? "No" : template.active_user_count}{" "}
          developer
          {template.active_user_count === 1 ? "" : "s"}
        </span>
      </div>
    </PopoverLink>
  );
};

const PopoverLink: FC<RouterLinkProps> = ({ children, ...linkProps }) => {
  return (
    <RouterLink
      {...linkProps}
      css={(theme) => ({
        color: theme.palette.text.primary,
        padding: "8px 16px",
        fontSize: 14,
        outline: "none",
        textDecoration: "none",
        "&:focus": {
          backgroundColor: theme.palette.action.focus,
        },
        "&:hover": {
          textDecoration: "none",
          backgroundColor: theme.palette.action.hover,
        },
      })}
    >
      {children}
    </RouterLink>
  );
};

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
    .filter(
      (template) =>
        termMatcher.test(template.display_name) ||
        termMatcher.test(template.name),
    )
    .sort((t1, t2) => t2.active_user_count - t1.active_user_count)
    .slice(0, 10);
}
