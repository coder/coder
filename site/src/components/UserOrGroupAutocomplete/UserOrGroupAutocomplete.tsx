import CircularProgress from "@mui/material/CircularProgress"
import { makeStyles } from "@mui/styles"
import TextField from "@mui/material/TextField"
import Autocomplete from "@mui/material/Autocomplete"
import { useMachine } from "@xstate/react"
import { Group, User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import debounce from "just-debounce-it"
import { ChangeEvent, useState } from "react"
import { getGroupSubtitle } from "utils/groups"
import { searchUsersAndGroupsMachine } from "xServices/template/searchUsersAndGroupsXService"
import Box from "@mui/material/Box"

export type UserOrGroupAutocompleteValue = User | Group | null

const isGroup = (value: UserOrGroupAutocompleteValue): value is Group => {
  return value !== null && "members" in value
}

export type UserOrGroupAutocompleteProps = {
  value: UserOrGroupAutocompleteValue
  onChange: (value: UserOrGroupAutocompleteValue) => void
  organizationId: string
  exclude: UserOrGroupAutocompleteValue[]
}

export const UserOrGroupAutocomplete: React.FC<
  UserOrGroupAutocompleteProps
> = ({ value, onChange, organizationId, exclude }) => {
  const styles = useStyles()
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const [searchState, sendSearch] = useMachine(searchUsersAndGroupsMachine, {
    context: {
      userResults: [],
      groupResults: [],
      organizationId,
    },
  })
  const { userResults, groupResults } = searchState.context
  const options = [...groupResults, ...userResults].filter((result) => {
    const excludeIds = exclude.map((optionToExclude) => optionToExclude?.id)
    return !excludeIds.includes(result.id)
  })

  const handleFilterChange = debounce(
    (event: ChangeEvent<HTMLInputElement>) => {
      sendSearch("SEARCH", { query: event.target.value })
    },
    500,
  )

  return (
    <Autocomplete
      value={value}
      id="user-or-group-autocomplete"
      open={isAutocompleteOpen}
      onOpen={() => {
        setIsAutocompleteOpen(true)
      }}
      onClose={() => {
        setIsAutocompleteOpen(false)
      }}
      onChange={(_, newValue) => {
        if (newValue === null) {
          sendSearch("CLEAR_RESULTS")
        }

        onChange(newValue)
      }}
      isOptionEqualToValue={(option, value) => option.id === value.id}
      getOptionLabel={(option) =>
        isGroup(option) ? option.name : option.email
      }
      renderOption={(props, option) => {
        const isOptionGroup = isGroup(option)

        return (
          <Box component="li" {...props}>
            <AvatarData
              title={isOptionGroup ? option.name : option.username}
              subtitle={isOptionGroup ? getGroupSubtitle(option) : option.email}
              src={option.avatar_url}
            />
          </Box>
        )
      }}
      options={options}
      loading={searchState.matches("searching")}
      className={styles.autocomplete}
      renderInput={(params) => (
        <>
          {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment -- Need it */}
          {/* @ts-ignore -- Issue from lib https://github.com/i18next/react-i18next/issues/1543 */}
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
  )
}

export const useStyles = makeStyles(() => {
  return {
    autocomplete: {
      width: "300px",

      "& .MuiFormControl-root": {
        width: "100%",
      },

      "& .MuiInputBase-root": {
        width: "100%",
      },
    },
  }
})
