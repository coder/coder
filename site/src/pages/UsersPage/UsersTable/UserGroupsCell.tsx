import { type PointerEvent, useId, useState } from "react";
import { useTheme } from "@emotion/react";
import { type Group } from "api/typesGenerated";

import { Stack } from "components/Stack/Stack";
import { Avatar } from "components/Avatar/Avatar";
import { OverflowY } from "components/OverflowY/OverflowY";

import TableCell from "@mui/material/TableCell";
import Button from "@mui/material/Button";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import Popover from "@mui/material/Popover";

type GroupsCellProps = {
  userGroups: readonly Group[] | undefined;
};

export function UserGroupsCell({ userGroups }: GroupsCellProps) {
  const hookId = useId();
  const theme = useTheme();

  // 2023-10-18 - Temporary code - waiting for Bruno to finish the refactoring
  // of PopoverContainer. Popover code should all be torn out and replaced with
  // PopoverContainer once the new API is ready
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);

  const closePopover = () => setAnchorEl(null);
  const openPopover = (event: PointerEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const isPopoverOpen = anchorEl !== null;
  const popoverId = `${hookId}-popover`;

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
            aria-haspopup
            aria-owns={isPopoverOpen ? popoverId : undefined}
            onPointerEnter={openPopover}
            onPointerLeave={closePopover}
            css={{
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
                  textDecoration: isPopoverOpen ? "none" : "underline",
                  textUnderlineOffset: "0.2em",
                }}
              >
                See details
              </span>
            </Stack>
          </Button>

          <Popover
            id={popoverId}
            anchorEl={anchorEl}
            open={anchorEl !== null}
            onClose={closePopover}
            anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
            disablePortal
            disableScrollLock
            css={{ pointerEvents: "none" }}
          >
            <OverflowY maxHeight={400} sx={{ maxWidth: "320px" }}>
              <List
                component="ul"
                css={{
                  display: "flex",
                  flexFlow: "column nowrap",
                  fontSize: theme.typography.body2.fontSize,
                  padding: theme.spacing(0.5, 0.25),
                  gap: theme.spacing(0),
                }}
              >
                {userGroups.map((group) => {
                  const groupName = group.display_name || group.name;
                  return (
                    <ListItem
                      key={group.id}
                      css={{
                        columnGap: theme.spacing(1.25),
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
          </Popover>
        </>
      )}
    </TableCell>
  );
}
