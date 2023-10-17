import TableCell from "@mui/material/TableCell";
import { Stack } from "components/Stack/Stack";
import { type Group } from "api/typesGenerated";

type GroupsCellProps = {
  groups: readonly Group[] | undefined;
};

export function GroupsCell({ groups }: GroupsCellProps) {
  return (
    <TableCell>
      {groups === undefined ? (
        <em>N/A</em>
      ) : (
        <Stack>
          <div>
            {groups.length} Group{groups.length !== 1 && "s"}
          </div>
        </Stack>
      )}
    </TableCell>
  );
}
