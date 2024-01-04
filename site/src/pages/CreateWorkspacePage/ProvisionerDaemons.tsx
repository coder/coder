import Person from "@mui/icons-material/Person";
import Public from "@mui/icons-material/Public";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import { ProvisionerDaemon } from "api/typesGenerated";
import { FC, useMemo } from "react";

export interface ProvisionerGroupSelectProps {
  groups: ProvisionerDaemon[][];
  onSelectGroup: (group: ProvisionerDaemon[]) => void;
}

export const ProvisionerGroupSelect: FC<ProvisionerGroupSelectProps> = ({
  groups,
  onSelectGroup,
}) => {
  return (
    <Select
      defaultValue="0"
      size="small"
      onChange={async (e) => {
        const index = parseInt(e.target.value);
        onSelectGroup(groups[index]);
      }}
      renderValue={(value) => {
        const daemons = groups[parseInt(value)];
        return provisionerGroupDisplayName(daemons);
      }}
    >
      {groups.map((group, index) => {
        // This shouldn't be possible, but is a safeguard.
        if (group.length === 0) {
          return null;
        }
        const daemon = group[0];
        // If the daemon is scoped to the organization, we should
        // give it the global appearance.
        const isGlobal = daemon.tags["scope"] === "organization";
        const displayName = provisionerGroupDisplayName(group);

        return (
          <MenuItem key={index} value={index}>
            {isGlobal ? <Public /> : <Person />}
            {displayName}
          </MenuItem>
        );
      })}
    </Select>
  );
};

const provisionerGroupDisplayName = (group: ProvisionerDaemon[]): string => {
  const daemon = group[0];
  // If the daemon is scoped to the organization, we should
  // give it the global appearance.
  const isGlobal = daemon.tags["scope"] === "organization";
  let displayName = "Global";
  if (!isGlobal) {
    displayName = "Personal";
    if (group.length === 1) {
      displayName = daemon.tags["hostname"] || daemon.name;
    }
  }
  return displayName;
};

export const useProvisionerDaemonGroups = (
  provisionerDaemons: ProvisionerDaemon[],
) => {
  return useMemo((): ProvisionerDaemon[][] => {
    // Group provisioner daemons that have the same sets of tags
    const groups: ProvisionerDaemon[][] = [];
    for (const daemon of provisionerDaemons) {
      // Ignore daemons that aren't actively connected!
      if (daemon.disconnected_at && daemon.last_seen_at) {
        if (new Date(daemon.disconnected_at) > new Date(daemon.last_seen_at)) {
          continue;
        }
      }

      const group = groups.find((group) =>
        Object.entries(group[0].tags).every(
          ([key, value]) => daemon.tags[key] === value,
        ),
      );
      if (group) {
        group.push(daemon);
      } else {
        groups.push([daemon]);
      }
    }
    return groups.sort((a, b) => b.length - a.length);
  }, [provisionerDaemons]);
};
