import { useTheme } from "@emotion/react";
import GroupIcon from "@mui/icons-material/Group";
import Button from "@mui/material/Button";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import TableCell from "@mui/material/TableCell";
import type { Group } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { OverflowY } from "components/OverflowY/OverflowY";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";

type GroupsCellProps = {
  userGroups: readonly Group[] | undefined;
};

export function UserGroupsCell({ userGroups }: GroupsCellProps) {
  const theme = useTheme();

  return (
    <TableCell>
      {userGroups === undefined ? (
        // Felt right to add emphasis to the undefined state for semantics
        // ("hey, this isn't normal"), but the default italics looked weird in
        // the table UI
        <em css={{ fontStyle: "normal" }}>N/A</em>
      ) : (
        <Popover mode="hover">
          <PopoverTrigger>
            <Button
              css={{
                justifyContent: "flex-start",
                fontSize: theme.typography.body2.fontSize,
                lineHeight: theme.typography.body2.lineHeight,
                fontWeight: 400,
                border: "none",
                padding: 0,
                "&:hover": {
                  border: "none",
                  backgroundColor: "transparent",
                },
              }}
            >
              <Stack
                spacing={0}
                direction="row"
                css={{ columnGap: 8, alignItems: "center" }}
              >
                <GroupIcon
                  css={{
                    width: "1rem",
                    height: "1rem",
                    opacity: userGroups.length > 0 ? 0.8 : 0.5,
                  }}
                />

                <span>
                  {userGroups.length} Group{userGroups.length !== 1 && "s"}
                </span>
              </Stack>
            </Button>
          </PopoverTrigger>

          <PopoverContent
            disableScrollLock
            disableRestoreFocus
            css={{
              ".MuiPaper-root": {
                minWidth: "auto",
              },
            }}
            anchorOrigin={{
              vertical: "top",
              horizontal: "center",
            }}
            transformOrigin={{
              vertical: "bottom",
              horizontal: "center",
            }}
          >
            <OverflowY maxHeight={400}>
              <List
                component="ul"
                css={{
                  display: "flex",
                  flexFlow: "column nowrap",
                  fontSize: theme.typography.body2.fontSize,
                  padding: "4px 2px",
                  gap: 0,
                }}
              >
                {userGroups.map((group) => {
                  const groupName = group.display_name || group.name;
                  return (
                    <ListItem
                      key={group.id}
                      css={{
                        columnGap: 10,
                        alignItems: "center",
                      }}
                    >
                      <Avatar size="xs" src={group.avatar_url} alt={groupName}>
                        {groupName}
                      </Avatar>

                      <span
                        css={{
                          whiteSpace: "nowrap",
                          textOverflow: "ellipsis",
                          overflow: "hidden",
                          lineHeight: 1,
                          margin: 0,
                        }}
                      >
                        {groupName || <em>N/A</em>}
                      </span>
                    </ListItem>
                  );
                })}
              </List>
            </OverflowY>
          </PopoverContent>
        </Popover>
      )}
    </TableCell>
  );
}
