import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  createOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import { myOrganizations } from "api/queries/users";

const TeamsSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const addTeamMutation = useMutation(createOrganization(queryClient));
  const deleteTeamMutation = useMutation(deleteOrganization(queryClient));
  const organizationsQuery = useQuery(myOrganizations());
  const [newOrgName, setNewOrgName] = useState("");
  return (
    <>
      <TextField
        label="New organization name"
        onChange={(event) => setNewOrgName(event.target.value)}
      />
      <p>{String(addTeamMutation.error)}</p>
      <Button onClick={() => addTeamMutation.mutate({ name: newOrgName })}>
        add new team
      </Button>

      <div>{String(deleteTeamMutation.error)}</div>

      {organizationsQuery.data?.map((org) => (
        <div key={org.id}>
          {org.name}{" "}
          <Button onClick={() => deleteTeamMutation.mutate(org.id)}>
            Delete
          </Button>
        </div>
      ))}
    </>
  );
};

export default TeamsSettingsPage;
