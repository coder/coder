import TableCell from "@mui/material/TableCell";
import { GroupsByUserId } from "api/queries/groups";
import { User } from "api/typesGenerated";

type GroupsCellProps = {
  user: User;
  groupsByUserId: GroupsByUserId | undefined;
};

export function GroupsCell({ user, groupsByUserId }: GroupsCellProps) {
  return <TableCell>5 Groups</TableCell>;
}
