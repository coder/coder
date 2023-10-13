import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import { useMachine } from "@xstate/react";
import Box from "@mui/material/Box";
import { type ChangeEvent, useState } from "react";
import { css } from "@emotion/react";
import type { Group, User } from "api/typesGenerated";
import { AvatarData } from "components/AvatarData/AvatarData";
import { getGroupSubtitle } from "utils/groups";
import { searchUsersAndGroupsMachine } from "xServices/template/searchUsersAndGroupsXService";
import { useDebouncedFunction } from "hooks/debounce";

export type UserOrGroupAutocompleteValue = User | Group | null;

const isGroup = (value: UserOrGroupAutocompleteValue): value is Group => {
  return value !== null && "members" in value;
};

export type UserOrGroupAutocompleteProps = {
  value: UserOrGroupAutocompleteValue;
  onChange: (value: UserOrGroupAutocompleteValue) => void;
  organizationId: string;
  templateID?: string;
  exclude: UserOrGroupAutocompleteValue[];
};

const autoCompleteStyles = css`
  width: 300px;

  & .MuiFormControl-root {
    width: 100%;
  }

  & .MuiInputBase-root {
    width: 100%;
  }
`;

export const UserOrGroupAutocomplete: React.FC<
  UserOrGroupAutocompleteProps
> = ({ value, onChange, organizationId, templateID, exclude }) => {
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
  const [searchState, sendSearch] = useMachine(searchUsersAndGroupsMachine, {
    context: {
      userResults: [],
      groupResults: [],
      organizationId,
      templateID,
    },
  });
  const { userResults, groupResults } = searchState.context;
  const options = [...groupResults, ...userResults].filter((result) => {
    const excludeIds = exclude.map((optionToExclude) => optionToExclude?.id);
    return !excludeIds.includes(result.id);
  });

  const { debounced: handleFilterChange } = useDebouncedFunction(
    (event: ChangeEvent<HTMLInputElement>) => {
      sendSearch("SEARCH", { query: event.target.value });
    },
    500,
  );

  return (
    <Autocomplete
      value={value}
      id="user-or-group-autocomplete"
      open={isAutocompleteOpen}
      onOpen={() => {
        setIsAutocompleteOpen(true);
      }}
      onClose={() => {
        setIsAutocompleteOpen(false);
      }}
      onChange={(_, newValue) => {
        if (newValue === null) {
          sendSearch("CLEAR_RESULTS");
        }

        onChange(newValue);
      }}
      isOptionEqualToValue={(option, value) => option.id === value.id}
      getOptionLabel={(option) =>
        isGroup(option) ? option.display_name || option.name : option.email
      }
      renderOption={(props, option) => {
        const isOptionGroup = isGroup(option);

        return (
          <Box component="li" {...props}>
            <AvatarData
              title={
                isOptionGroup
                  ? option.display_name || option.name
                  : option.username
              }
              subtitle={isOptionGroup ? getGroupSubtitle(option) : option.email}
              src={option.avatar_url}
            />
          </Box>
        );
      }}
      options={options}
      loading={searchState.matches("searching")}
      css={autoCompleteStyles}
      renderInput={(params) => (
        <>
          <TextField
            {...params}
            margin="none"
            size="small"
            placeholder="Search for user or group"
            InputProps={{
              ...params.InputProps,
              onChange: handleFilterChange,
              endAdornment: (
                <>
                  {searchState.matches("searching") ? (
                    <CircularProgress size={16} />
                  ) : null}
                  {params.InputProps.endAdornment}
                </>
              ),
            }}
          />
        </>
      )}
    />
  );
};
