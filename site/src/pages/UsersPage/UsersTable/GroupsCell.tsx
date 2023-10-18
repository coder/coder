import { useState } from "react";
import { useTheme } from "@emotion/react";
import { type Group } from "api/typesGenerated";

import { Stack } from "components/Stack/Stack";
import { Avatar } from "components/Avatar/Avatar";
import TableCell from "@mui/material/TableCell";
import Button from "@mui/material/Button";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText from "@mui/material/ListItemText";
import ListItemAvatar from "@mui/material/ListItemAvatar";
import Popover from "@mui/material/Popover";

type GroupsCellProps = {
  userGroups: readonly Group[] | undefined;
};

export function GroupsCell({ userGroups }: GroupsCellProps) {
  const [isHovering, setIsHovering] = useState(false);
  const theme = useTheme();

  // 2023-10-18 - Temporary code - waiting for Bruno to finish the refactoring
  // of PopoverContainer. Popover code should all be torn out and replaced with
  // PopoverContainer once the new API is ready
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const hideCtaUnderline = isHovering || anchorEl !== null;

  return (
    <TableCell>
      {userGroups === undefined ? (
        // Felt right to add emphasis to the undefined state for semantics
        // ("hey, this isn't normal"), but the default italics looked weird in
        // the table UI
        <em css={{ fontStyle: "normal" }}>N/A</em>
      ) : (
        <>
          <Button
            onPointerEnter={() => setIsHovering(true)}
            onPointerLeave={() => setIsHovering(false)}
            onClick={(event) => setAnchorEl(event.currentTarget)}
            css={{
              width: "100%",
              justifyContent: "flex-start",
              fontSize: theme.typography.body1.fontSize,
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
            <Stack spacing={0}>
              <span>
                {userGroups.length} Group{userGroups.length !== 1 && "s"}
              </span>

              <span
                css={{
                  fontSize: "0.75rem",
                  color: theme.palette.text.secondary,
                  textDecoration: hideCtaUnderline ? "none" : "underline",
                  textUnderlineOffset: "0.2em",
                }}
              >
                See details
              </span>
            </Stack>
          </Button>

          <Popover
            anchorEl={anchorEl}
            open={anchorEl !== null}
            onClose={() => setAnchorEl(null)}
            anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
            transformOrigin={{ vertical: 16, horizontal: 0 }}
          >
            <List
              component="ul"
              css={{
                padding: theme.spacing(2),
              }}
            >
              {userGroups.map((group) => {
                const groupText = group.display_name || group.name;
                return (
                  <ListItem key={group.id}>
                    <ListItemAvatar>
                      <Avatar src={group.avatar_url} alt={groupText} />
                    </ListItemAvatar>

                    <ListItemText>{groupText}</ListItemText>
                  </ListItem>
                );
              })}
            </List>
          </Popover>
        </>
      )}
    </TableCell>
  );
}
