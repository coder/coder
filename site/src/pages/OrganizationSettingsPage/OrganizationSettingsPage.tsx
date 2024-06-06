import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  createOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import { myOrganizations } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";

const OrganizationSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const addOrganizationMutation = useMutation(createOrganization(queryClient));
  const deleteOrganizationMutation = useMutation(
    deleteOrganization(queryClient),
  );
  const organizationsQuery = useQuery(myOrganizations());
  const [newOrgName, setNewOrgName] = useState("");

  const error =
    addOrganizationMutation.error ?? deleteOrganizationMutation.error;

  return (
    <Margins verticalMargin={48}>
      {Boolean(error) && <ErrorAlert error={error} />}

      <TextField
        label="New organization name"
        onChange={(event) => setNewOrgName(event.target.value)}
      />
      <Button
        onClick={() => addOrganizationMutation.mutate({ name: newOrgName })}
      >
        add new team
      </Button>

      {organizationsQuery.data?.map((org) => (
        <div key={org.id}>
          {org.name}{" "}
          <Button onClick={() => deleteOrganizationMutation.mutate(org.id)}>
            Delete
          </Button>
        </div>
      ))}
    </Margins>
  );
};

export default OrganizationSettingsPage;
