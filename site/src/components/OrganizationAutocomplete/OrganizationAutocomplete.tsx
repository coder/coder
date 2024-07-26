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

export type OrganizationAutocompleteProps = {
  value: Organization | null;
  onChange: (organization: Organization | null) => void;
  label?: string;
  className?: string;
  size?: ComponentProps<typeof TextField>["size"];
  required?: boolean;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
  value,
  onChange,
  label,
  className,
  size = "small",
  required,
}) => {
  const [autoComplete, setAutoComplete] = useState<{
    value: string;
    open: boolean;
  }>({
    value: value?.name ?? "",
    open: false,
  });
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
      noOptionsText="No organizations found"
      className={className}
      options={organizationsQuery.data ?? []}
      loading={organizationsQuery.isLoading}
      value={value}
      data-testid="organization-autocomplete"
      open={autoComplete.open}
      isOptionEqualToValue={(a, b) => a.name === b.name}
      getOptionLabel={(option) => option.display_name}
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
      renderOption={({ key, ...props }, option) => (
        <li key={key} {...props}>
          <AvatarData
            title={option.display_name}
            subtitle={option.name}
            src={option.icon}
          />
        </li>
      )}
      renderInput={(params) => (
        <TextField
          {...params}
          required={required}
          fullWidth
          size={size}
          label={label}
          autoFocus
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
                {organizationsQuery.isFetching && autoComplete.open && (
                  <CircularProgress size={16} />
                )}
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
