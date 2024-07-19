import { css } from "@emotion/css";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import {
  type ChangeEvent,
  type ComponentProps,
  type FC,
  useState,
} from "react";
import { useQuery } from "react-query";
import { organizations } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { useDebouncedFunction } from "hooks/debounce";
// import { prepareQuery } from "utils/filters";

export type OrganizationAutocompleteProps = {
  value: Organization | null;
  onChange: (organization: Organization | null) => void;
  label?: string;
  className?: string;
  size?: ComponentProps<typeof TextField>["size"];
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
  value,
  onChange,
  label,
  className,
  size = "small",
}) => {
  const [autoComplete, setAutoComplete] = useState<{
    value: string;
    open: boolean;
  }>({
    value: value?.name ?? "",
    open: false,
  });
  // const usersQuery = useQuery({
  //   ...users({
  //     q: prepareQuery(encodeURI(autoComplete.value)),
  //     limit: 25,
  //   }),
  //   enabled: autoComplete.open,
  //   keepPreviousData: true,
  // });
  const organizationsQuery = useQuery(organizations());

  const { debounced: debouncedInputOnChange } = useDebouncedFunction(
    (event: ChangeEvent<HTMLInputElement>) => {
      setAutoComplete((state) => ({
        ...state,
        value: event.target.value,
      }));
    },
    750,
  );

  return (
    <Autocomplete
      // Since the values are filtered by the API we don't need to filter them
      // in the FE because it can causes some mismatches.
      filterOptions={(organization) => organization}
      noOptionsText="No users found"
      className={className}
      options={organizationsQuery.data ?? []}
      loading={organizationsQuery.isLoading}
      value={value}
      id="organization-autocomplete"
      open={autoComplete.open}
      onOpen={() => {
        setAutoComplete((state) => ({
          ...state,
          open: true,
        }));
      }}
      onClose={() => {
        setAutoComplete({
          value: value?.name ?? "",
          open: false,
        });
      }}
      onChange={(_, newValue) => {
        onChange(newValue);
      }}
      isOptionEqualToValue={(option: Organization, value: Organization) =>
        option.name === value.name
      }
      getOptionLabel={(option) => option.name}
      renderOption={(props, option) => {
        const { key, ...optionProps } = props;
        return (
          <li key={key} {...optionProps}>
            <AvatarData
              title={option.name}
              subtitle={option.display_name}
              src={option.icon}
            />
          </li>
        );
      }}
      renderInput={(params) => (
        <TextField
          {...params}
          fullWidth
          size={size}
          label={label}
          placeholder="Organization name"
          css={{
            "&:not(:has(label))": {
              margin: 0,
            },
          }}
          InputProps={{
            ...params.InputProps,
            onChange: debouncedInputOnChange,
            startAdornment: value && (
              <Avatar size="sm" src={value.icon}>
                {value.name}
              </Avatar>
            ),
            endAdornment: (
              <>
                {organizationsQuery.isFetching && autoComplete.open ? (
                  <CircularProgress size={16} />
                ) : null}
                {params.InputProps.endAdornment}
              </>
            ),
            classes: { root },
          }}
          InputLabelProps={{
            shrink: true,
          }}
        />
      )}
    />
  );
};

const root = css`
  padding-left: 14px !important; // Same padding left as input
  gap: 4px;
`;
